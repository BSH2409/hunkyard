import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage, getMdPath, switchToDocumentView, openDocLineComment, submitReview } from './helpers';

test.describe('Return feedback to the launching CLI', () => {
  test.beforeEach(async ({ request }) => { await clearAllComments(request); });

  test('line comments are saved and explicitly published without spawning an agent', async ({ page }) => {
    await loadPage(page);
    const doc = await switchToDocumentView(page);
    const textarea = await openDocLineComment(page, doc);
    await expect(page.locator('.btn-agent')).toHaveCount(0);
    await textarea.fill('Please review this section');
    await page.locator('.comment-form .btn-primary').click();
    const publication = page.waitForResponse(r => r.url().includes('/api/finish') && r.request().method() === 'POST');
    await submitReview(page, 'request_changes');
    const result = await (await publication).json();
    expect(result.batch.disposition).toBe('request_changes');
    expect(result.batch.comments.some((c: {body: string}) => c.body === 'Please review this section')).toBe(true);
  });

  test('the subprocess agent endpoint is disabled', async ({ request }) => {
    const mdPath = await getMdPath(request);
    const comment = await request.post(`/api/file/comments?path=${encodeURIComponent(mdPath)}`, { data: { start_line: 5, end_line: 5, body: 'Is this safe?' } }).then(r => r.json());
    const response = await request.post('/api/agent/request', { data: { comment_id: comment.id, file_path: mdPath } });
    expect(response.status()).toBe(403);
  });

  test('review-level feedback is included in the published host batch', async ({ page }) => {
    await loadPage(page);
    await page.keyboard.press('Shift+G');
    const textarea = page.locator('#reviewConversation .comment-form textarea');
    await textarea.fill('Describe the problem and how we fix it');
    await expect(page.locator('#reviewConversation .btn-agent')).toHaveCount(0);
    await page.locator('#reviewConversation .btn-primary').click();
    await expect(page.locator('#reviewConversation .comment-card')).toContainText('Describe the problem and how we fix it');
    const publication = page.waitForResponse(r => r.url().includes('/api/finish') && r.request().method() === 'POST');
    await submitReview(page);
    const result = await (await publication).json();
    expect(result.batch.comments.some((c: {body: string; scope: string}) => c.scope === 'review' && c.body === 'Describe the problem and how we fix it')).toBe(true);
  });
});
