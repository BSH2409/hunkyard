import { test, expect, type Locator } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import {
  clearAllComments, loadPage, addComment, getReviewFilePath, revealFile,
  waitForScrollStable, waitUntilHittable, reviewScroller, signalRoundComplete,
  submitReview, submitReviewDialog } from './helpers';

// Rebuilding every file section hands back deferred (empty) bodies, so the
// document collapses shorter than the current offset and the browser clamps the
// scroll to the top. Replying and deleting used to trigger that via the
// comments-changed SSE event. The review list scrolls inside #filesContainer
// (Pierre CodeView), so offsets below are that pane's scrollTop.

// Pane scroll offset plus the element's on-screen top.
function measure(el: Locator) {
  return el.evaluate(node => ({
    scrollTop: document.getElementById('filesContainer')!.scrollTop,
    top: node.getBoundingClientRect().top,
  }));
}
test.describe('Scroll position across comment updates', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllComments(request);
  });

  test('submitting a reply keeps the thread where it was on screen', async ({ page, request }) => {
    // Small viewport so most file bodies stay deferred while we read one thread.
    await page.setViewportSize({ width: 1200, height: 400 });

    const session = await (await request.get('/api/session')).json();
    const lastFile = session.files[session.files.length - 1].path as string;
    await addComment(request, lastFile, 5, 'Reply to me');

    await loadPage(page);

    const item = await revealFile(page, lastFile);
    const card = item.locator('.comment-card').first();
    await expect(card).toBeVisible();

    // Park the thread mid-viewport and let virtualized items settle so the
    // measurement below isn't racing the mount.
    await card.evaluate(el => el.scrollIntoView({ block: 'center', behavior: 'instant' }));
    await waitForScrollStable(page);
    await waitUntilHittable(card.locator('.reply-input'));
    const before = await measure(card);
    expect(before.scrollTop).toBeGreaterThan(100);

    await card.locator('.reply-input').click();
    await card.locator('.reply-textarea').fill('replying without losing my place');
    await card.locator('.reply-form-buttons .btn-primary').click();

    await expect(card.locator('.comment-reply')).toHaveCount(1);
    // The jump happened when the SSE event landed, after the local re-render.
    await waitForScrollStable(page);

    const after = await measure(card);
    expect(after.scrollTop).toBeGreaterThan(100);
    expect(Math.abs(after.top - before.top)).toBeLessThan(50);
  });

  // Deleting broadcasts the same event, so it lost the reader's place too.
  test('deleting a comment keeps the surrounding file where it was on screen', async ({ page, request }) => {
    await page.setViewportSize({ width: 1200, height: 400 });

    const session = await (await request.get('/api/session')).json();
    const lastFile = session.files[session.files.length - 1].path as string;
    await addComment(request, lastFile, 5, 'Delete me');

    await loadPage(page);

    const section = await revealFile(page, lastFile);
    const card = section.locator('.comment-card').first();
    await expect(card).toBeVisible();

    await card.evaluate(el => el.scrollIntoView({ block: 'center', behavior: 'instant' }));
    await waitForScrollStable(page);
    await waitUntilHittable(card.locator('.comment-actions .delete-btn'));
    // The card itself disappears, so anchor the measurement to its file.
    const before = await measure(section);
    expect(before.scrollTop).toBeGreaterThan(100);

    await card.locator('.comment-actions .delete-btn').click();
    await expect(page.locator('.comment-card')).toHaveCount(0);
    await waitForScrollStable(page);

    const after = await measure(section);
    expect(after.scrollTop).toBeGreaterThan(100);
    expect(Math.abs(after.top - before.top)).toBeLessThan(50);
  });

  // Round-complete (the agent re-arming after Finish Review) re-fetches the
  // session and rebuilds every section, so it threw the reader back to the top
  // once per round — the same deferred-body collapse as above.
  test('round-complete keeps the file being read where it was on screen', async ({ page, request }) => {
    await page.setViewportSize({ width: 1200, height: 400 });

    const session = await (await request.get('/api/session')).json();
    const lastFile = session.files[session.files.length - 1].path as string;
    await addComment(request, lastFile, 5, 'Round me');

    await loadPage(page);

    const section = await revealFile(page, lastFile);
    await section.evaluate(el => el.scrollIntoView({ block: 'start', behavior: 'instant' }));
    await waitForScrollStable(page);
    const before = await measure(section);
    expect(before.scrollTop).toBeGreaterThan(100);

    await submitReview(page);
    const overlay = page.locator('#waitingOverlay');
    await expect(overlay).toHaveClass(/active/);

    await signalRoundComplete(request);
    await expect(overlay).not.toHaveClass(/active/, { timeout: 5_000 });
    await waitForScrollStable(page);

    // The rebuild may replace the item node; the locator re-resolves to the new one.
    const after = await measure(section);
    expect(after.scrollTop).toBeGreaterThan(100);
    expect(Math.abs(after.top - before.top)).toBeLessThan(50);
  });

  test('round-complete preserves the review room and file scroll positions', async ({ page, request }) => {
    await page.setViewportSize({ width: 1200, height: 400 });
    await expect(await request.post('/api/comments', {
      data: { body: 'Conversation\n\n' + 'A paragraph\n\n'.repeat(30) },
    })).toBeOK();
    const reviewPath = await getReviewFilePath(request);
    await loadPage(page);
    const pane = reviewScroller(page);
    await pane.evaluate(el => el.scrollTo({ top: 2000, behavior: 'instant' }));
    await waitForScrollStable(page);
    const fileScroll = await pane.evaluate(el => el.scrollTop);
    expect(fileScroll).toBeGreaterThan(100);

    await page.getByRole('button', { name: 'Open review conversation' }).click();
    const messages = page.getByRole('log', { name: 'Review messages' });
    const composer = page.getByRole('textbox', { name: 'Message the review room' });
    await expect(composer).toBeFocused();
    await expect(messages.getByText('Conversation', { exact: true })).toBeVisible();
    await messages.evaluate(el => el.scrollTo({ top: 120, behavior: 'instant' }));
    expect(await messages.evaluate(el => el.scrollTop)).toBe(120);
    expect(await pane.evaluate(el => el.scrollTop)).toBe(fileScroll);

    await page.locator('#reviewChatSend').click();
    await submitReviewDialog(page);
    const overlay = page.locator('#waitingOverlay');
    await expect(overlay).toHaveClass(/active/);

    // An agent can reply to the conversation on disk immediately before
    // re-arming. Round-complete imports it even before the file watcher polls.
    const reply = 'New reply during round completion';
    const review = JSON.parse(readFileSync(reviewPath, 'utf8'));
    review.review_comments[0].replies = [{
      id: 'rp_round_complete',
      author: 'Codex',
      author_kind: 'agent',
      body: reply,
      created_at: new Date().toISOString(),
    }];
    writeFileSync(reviewPath, JSON.stringify(review));
    await expect(await signalRoundComplete(request)).toBeOK();
    await expect(overlay).not.toHaveClass(/active/);
    await expect(messages.locator('.comment-reply')).toContainText(reply);
    await waitForScrollStable(page);

    await expect(messages).toBeVisible();
    await expect(composer).not.toBeFocused();
    expect(await messages.evaluate(el => el.scrollTop)).toBe(120);
    expect(await pane.evaluate(el => el.scrollTop)).toBe(fileScroll);
  });

  // Hide-resolved is CSS for cards plus a highlight sync — it must not wipe
  // #filesContainer. A full rebuild was both unnecessary and the scroll bug.
  test('toggling hide-resolved preserves file item DOM nodes', async ({ page }) => {
    await page.setViewportSize({ width: 1200, height: 400 });
    await loadPage(page);

    await reviewScroller(page).evaluate(el => el.scrollTo({ top: 2000, behavior: 'instant' }));
    await waitForScrollStable(page);

    const before = await page.evaluate(() => {
      const pane = document.getElementById('filesContainer')!;
      const headers = [...pane.querySelectorAll('.pierre-file-header[data-file-path]')] as HTMLElement[];
      const header = headers.find(h => h.closest('diffs-container')!.getBoundingClientRect().bottom > 0);
      if (!header) return null;
      const item = header.closest('diffs-container') as HTMLElement;
      // Stamp the live node so we can tell a rebuild from an in-place update.
      item.dataset.hunkyardPreserveProbe = '1';
      return { id: header.dataset.filePath!, top: item.getBoundingClientRect().top, scrollTop: pane.scrollTop };
    });
    expect(before).toBeTruthy();
    expect(before!.scrollTop).toBeGreaterThan(100);

    const wasHidden = await page.evaluate(() => document.body.classList.contains('hide-resolved'));
    await page.keyboard.press('h');
    // Hide-resolved is a CSS class toggle on <body> — wait for it to flip.
    await expect.poll(
      () => page.evaluate(() => document.body.classList.contains('hide-resolved')),
    ).toBe(!wasHidden);

    const after = await page.evaluate((id: string) => {
      const pane = document.getElementById('filesContainer')!;
      const header = [...pane.querySelectorAll('.pierre-file-header[data-file-path]')]
        .find(h => (h as HTMLElement).dataset.filePath === id);
      const item = header ? header.closest('diffs-container') as HTMLElement : null;
      return {
        sameNode: !!(item && item.dataset.hunkyardPreserveProbe === '1'),
        top: item ? item.getBoundingClientRect().top : null,
        scrollTop: pane.scrollTop,
      };
    }, before!.id);
    expect(after.sameNode).toBe(true);
    expect(after.scrollTop).toBeGreaterThan(100);
    expect(Math.abs((after.top ?? 0) - before!.top)).toBeLessThan(5);
  });
});
