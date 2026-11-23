import { test, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { clearAllComments, loadPage, submitReviewDialog, getReviewFilePath, readFixtureState } from './helpers';

test.beforeEach(async ({ request }) => { await clearAllComments(request); });

test('conversation code references navigate to the reviewed file and line', async ({ page, request }) => {
  await expect(await request.post('/api/comments', { data: { body: 'Please inspect `server.go:5` and @utils.go:3.' } })).toBeOK();
  await loadPage(page);
  await page.locator('#reviewChatToggle').click();
  const reference = page.locator('#reviewConversation .code-reference').filter({ hasText: 'server.go:5' });
  await expect(reference).toHaveCount(1);
  await expect(reference).toBeVisible();
  await reference.click();
  await expect(page.locator('.tree-file.active')).toHaveAttribute('data-tree-path', 'server.go');
  await expect(page.locator('.pierre-file-header[data-file-path="server.go"]')).toBeVisible();
  const second = page.locator('#reviewConversation .code-reference').filter({ hasText: '@utils.go:3' });
  await second.focus();
  await second.press('Enter');
  await expect(page.locator('.tree-file.active')).toHaveAttribute('data-tree-path', 'utils.go');
});

test('conversation fits a narrow viewport and closes with Escape', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await loadPage(page);
  await page.locator('#reviewChatToggle').click();
  const panel = page.locator('#reviewChatPanel');
  await expect(panel).toBeVisible();
  const box = await panel.boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(375);
  await page.locator('.review-chat-close').focus();
  await page.keyboard.press('Escape');
  await expect(panel).toBeHidden();
  await expect(page.locator('#reviewChatToggle')).toBeFocused();
});

test('reviewing changes reveals and focuses a draft in the closed chat', async ({ page }) => {
  await loadPage(page);
  await page.locator('#reviewChatToggle').click();
  const composer = page.locator('#reviewConversation .review-chat-composer textarea');
  await composer.fill('Keep this draft');
  await page.locator('.review-chat-close').click();
  await expect(page.locator('#reviewChatPanel')).toBeHidden();
  await page.locator('#finishBtn').click();
  await expect(page.locator('#reviewChatPanel')).toBeVisible();
  await expect(composer).toHaveValue('Keep this draft');
  await expect(composer).toBeFocused();
  await expect(page.locator('#publishReviewDialog')).toBeHidden();
});

test('conversation sends through the existing review publication flow', async ({ page, request }) => {
  await expect(await request.post('/api/comments', { data: { body: 'Please check the dependency' } })).toBeOK();
  await loadPage(page);
  await page.locator('#reviewChatToggle').click();
  await page.locator('#reviewChatSend').click();
  const response = page.waitForResponse(r => r.url().includes('/api/finish') && r.request().method() === 'POST');
  await submitReviewDialog(page, 'comment');
  const publication = await (await response).json();
  expect(publication.batch.comments.some((c: {body: string}) => c.body === 'Please check the dependency')).toBe(true);
  await expect(page.locator('#reviewChatSend')).toBeEnabled();
  await page.locator('#backToEditing').click();
  await page.locator('#reviewChatSend').click();
  await expect(page.locator('#publishReviewDialog')).toBeVisible();
  await page.locator('#publishReviewCancel').click();
});

test('resizing from the upper-left keeps the room anchored on the right', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await loadPage(page);
  await page.locator('#reviewChatToggle').click();
  const panel = page.locator('#reviewChatPanel');
  const before = await panel.boundingBox();
  const handle = page.getByRole('button', { name: 'Resize conversation', exact: true });
  const grip = await handle.boundingBox();
  expect(grip!.x).toBeLessThan(before!.x + 10);
  await page.mouse.move(grip!.x + grip!.width / 2, grip!.y + grip!.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip!.x - 80, grip!.y - 60, { steps: 6 });
  await page.mouse.up();
  const after = await panel.boundingBox();
  expect(after!.width).toBeGreaterThan(before!.width + 50);
  expect(after!.height).toBeGreaterThan(before!.height + 30);
  expect(Math.abs(after!.x + after!.width - before!.x - before!.width)).toBeLessThan(2);
});

test('agents must be explicitly invited before a group request', async ({ page, request }) => {
  await loadPage(page);
  await page.locator('#reviewChatToggle').click();
  await expect(page.locator('.review-chat-composer-actions')).toContainText('Send to the terminal session that opened this review');
  const claude = page.getByRole('button', { name: 'Ask Claude', exact: true });
  const codex = page.getByRole('button', { name: 'Ask Codex', exact: true });
  await expect(claude).toBeVisible();
  await expect(codex).toBeVisible();
  await expect(claude.locator('img').first()).toHaveAttribute('src', 'images/integrations/claude-code-dark.svg');
  await claude.click();
  await expect.poll(async () => (await (await request.get('/api/discussion')).json()).participants).toContain('claude');
  await expect(claude).toHaveAttribute('aria-pressed', 'true');
  await codex.click();
  await expect(codex).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('.review-chat-heading')).toContainText('You · Claude · Codex');
  await expect(page.locator('.review-chat-composer-actions')).toContainText('Send to Claude + Codex');
  const composer = page.getByRole('textbox', { name: 'Message the review room', exact: true });
  await composer.fill('@Claude can you explain this?');
  await expect(page.locator('.review-chat-composer-actions')).toContainText('Send to Claude');
  await expect(page.locator('.review-chat-composer-actions')).not.toContainText('Codex');
  await claude.click();
  await expect(claude).toHaveAttribute('aria-pressed', 'false');
  await expect(composer).toHaveValue('@Claude can you explain this?');
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeDisabled();
  await composer.clear();
  await expect((await (await request.get('/api/discussion')).json()).requests).toHaveLength(0);
  await page.evaluate(async () => {
    await fetch('/api/discussion', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ participants: [] }) });
  });
});

test('both agents appear on the left and unread replies badge the launcher', async ({ page, request }) => {
  const parent = await (await request.post('/api/comments', { data: { body: 'Discuss the staged dependency', author: 'Reviewer' } })).json();
  const reviewPath = await getReviewFilePath(request);
  await expect(() => { expect(fs.readFileSync(reviewPath, 'utf8')).toContain(parent.id); }).toPass();
  const fixture = readFixtureState();
  const home = fixture.FAKE_HOME;
  const key = path.basename(path.dirname(reviewPath));
  await loadPage(page);
  const replyFromAgent = (author: string, body: string) => execFileSync(fixture.HUNKYARD_BIN, ['comment', '--session', key, '--reply-to', parent.id, '--author', author, body], {
    cwd: fixture.HUNKYARD_FIXTURE_DIR,
    env: { ...process.env, HOME: home, USERPROFILE: home, HUNKYARD_OWNER_ID: `codex:e2e:${home}` },
    timeout: 10_000,
  });
  for (const author of ['Claude', 'Codex']) replyFromAgent(author, author + ' review advice');
  await expect(page.locator('#reviewChatUnread')).toHaveText('2');
  await expect(page.locator('#reviewChatUnread')).toBeVisible();
  await page.locator('#reviewChatToggle').click();
  await expect(page.locator('.review-chat-row--sent')).toHaveCount(1);
  await expect(page.locator('.review-chat-row--received')).toHaveCount(2);
  await expect(page.locator('.review-chat-agent-badge')).toHaveCount(2);
  const sent = await page.locator('.review-chat-row--sent .review-chat-message').boundingBox();
  const received = await page.locator('.review-chat-row--received .review-chat-message').first().boundingBox();
  expect(sent!.x).toBeGreaterThan(received!.x);
  await expect(page.locator('#reviewChatUnread')).toBeHidden();
  const composer = page.getByRole('textbox', { name: 'Message the review room', exact: true });
  await composer.fill('Draft while agents reply');
  replyFromAgent('Claude', 'A follow-up while you type');
  await expect(page.locator('.review-chat-row--received')).toHaveCount(3);
  await expect(composer).toHaveValue('Draft while agents reply');
  await expect(composer).toBeFocused();
  await page.locator('.review-chat-close').click();
  await page.reload();
  await expect(page.locator('#reviewChatUnread')).toBeHidden();
});
