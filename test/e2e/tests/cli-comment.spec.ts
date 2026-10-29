import { test, expect } from '@playwright/test';
import { execSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import {
  clearAllComments, addComment, loadPage, switchToDocumentView, getReviewFilePath, readFixtureState, publishFixtureReview } from './helpers';

// Run `hunkyard comment <args>` in the fixture dir, against the fixture's daemon.
function hunkyardComment(args: string) {
  const env = readFixtureState();
  const hunkyardBin = env['HUNKYARD_BIN'];
  const fixtureDir = env['HUNKYARD_FIXTURE_DIR'];
  const fakeHome = env['FAKE_HOME'];
  if (!hunkyardBin || !fixtureDir || !fakeHome) {
    throw new Error('HUNKYARD_BIN, HUNKYARD_FIXTURE_DIR, or FAKE_HOME not set in state file');
  }
  // On Windows, Go's os.UserHomeDir() reads USERPROFILE, not HOME — so the
  // spawned `hunkyard comment` would otherwise look for the daemon registry in
  // the runner's real home and miss the fixture's daemon.
  execSync(`"${hunkyardBin}" comment ${args}`, {
    shell: true, timeout: 5000, cwd: fixtureDir,
    env: { ...process.env, HOME: fakeHome, USERPROFILE: fakeHome, CODEX_HOME: path.join(fakeHome, '.codex'), CLAUDE_CONFIG_DIR: path.join(fakeHome, '.claude'), HUNKYARD_OWNER_ID: `codex:e2e:${fakeHome}` },
  } as const);
}

test.describe('CLI comment sync — live browser update', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllComments(request);
  });

  test('hunkyard comment adds a comment and updates the header badge via SSE', async ({ page }) => {
    await loadPage(page);
    const doc = await switchToDocumentView(page);
    const badgeEl = page.locator('#commentCountNumber');

    // Wait for the document to be stable before asserting there are no comments.
    await expect(doc.locator('.line-block').first()).toBeVisible();
    await expect(doc.locator('.comment-card')).toHaveCount(0);
    await expect(badgeEl).toHaveText('');

    hunkyardComment('plan.md:1 "Hello from CLI"');

    // SSE triggers a re-fetch: the card appears and the badge counts it.
    await expect(doc.locator('.comment-body')).toContainText('Hello from CLI');
    await expect(page.locator('#commentCount')).toBeVisible();
    await expect(badgeEl).toHaveText('1');
  });

  test('hunkyard comment --clear removes all comments in the browser via SSE', async ({ page, request }) => {
    await addComment(request, 'plan.md', 1, 'Comment to be cleared');
    await loadPage(page);
    const doc = await switchToDocumentView(page);
    await expect(doc.locator('.comment-body')).toContainText('Comment to be cleared');

    // Wait until the review file on disk holds the comment (debounce fired):
    // --clear works on that file.
    await publishFixtureReview(request);
    const reviewFilePath = await getReviewFilePath(request);
    await expect(async () => {
      expect(fs.readFileSync(reviewFilePath, 'utf8')).toContain('Comment to be cleared');
    }).toPass();

    hunkyardComment('--clear');

    // SSE triggers a re-fetch; no comment cards remain.
    await expect(doc.locator('.comment-card')).toHaveCount(0);
  });

  test('a reply being typed keeps focus when an agent reply arrives via SSE', async ({ page, request }) => {
    const comment = await addComment(request, 'plan.md', 1, 'Reviewer comment');
    await loadPage(page);
    const doc = await switchToDocumentView(page);

    const card = doc.locator('.comment-card[data-comment-id="' + comment.id + '"]');
    await card.locator('.reply-input').click();
    const replyTextarea = card.locator('.reply-textarea');
    await expect(replyTextarea).toBeFocused();
    await page.keyboard.type('draft reply');

    // An agent reply re-renders the card through comments-changed while the reviewer is mid-sentence.
    const agentReply = await request.post(
      `/api/comment/${comment.id}/replies?path=${encodeURIComponent('plan.md')}`,
      { data: { body: 'Agent reply', author: 'Agent' } },
    );
    expect(agentReply.ok()).toBeTruthy();
    await expect(card.locator('.reply-body')).toContainText('Agent reply');

    await expect(replyTextarea).toBeFocused();
    await page.keyboard.type(' continues');
    await expect(replyTextarea).toHaveValue('draft reply continues');
  });
});
