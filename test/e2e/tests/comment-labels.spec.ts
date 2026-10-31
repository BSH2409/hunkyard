import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage, goSection, openLineComment } from './helpers';

test.beforeEach(async ({ request }) => { await clearAllComments(request); });

test('a blocking issue label shows on the card and holds back Approve until resolved', async ({ page, request }) => {
  await loadPage(page);
  const section = await goSection(page);
  const form = await openLineComment(page, section, 5);
  await form.locator('.comment-label-chip[data-label="issue"]').click();
  await form.locator('.comment-label-chip[data-blocking="true"]').click();
  await form.locator('textarea').fill('Use the shared client here.');
  const saved = page.waitForResponse(r => r.url().includes('/api/file/comments?') && r.request().method() === 'POST');
  await form.locator('.btn-primary').click();
  const created = await (await saved).json();
  expect(created.label).toBe('issue');
  expect(created.blocking).toBe(true);

  const card = page.locator('.comment-card', { hasText: 'Use the shared client here.' });
  await expect(card.locator('.comment-label--issue')).toHaveText('issue');
  await expect(card.locator('.comment-label--blocking')).toHaveText('blocking');

  await page.locator('#finishBtn').click();
  const dialog = page.getByRole('dialog', { name: 'Submit review', exact: true });
  await expect(dialog.locator('input[value="approve"]')).toBeDisabled();
  await expect(dialog.locator('#publishApproveBlocked')).toHaveText('1 blocking comment is unresolved.');
  await page.keyboard.press('Escape');

  const comments = await (await request.get('/api/files/comments')).json();
  const [filePath, list] = Object.entries(comments as Record<string, { id: string; body: string }[]>).find(([, l]) => l.some(c => c.body === 'Use the shared client here.'))!;
  const id = list.find(c => c.body === 'Use the shared client here.')!.id;
  expect((await request.put(`/api/comment/${id}/resolve?path=${encodeURIComponent(filePath)}`, { data: { resolved: true } })).ok()).toBeTruthy();
  await loadPage(page);
  await page.locator('#finishBtn').click();
  await expect(dialog.locator('input[value="approve"]')).toBeEnabled();
  await page.keyboard.press('Escape');
});
