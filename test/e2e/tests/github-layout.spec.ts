import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage, revealFile, goSection, switchToDocumentView } from './helpers';

test.beforeEach(async ({ request }) => { await clearAllComments(request); });

test('a fresh review includes all changes and the file filter preserves the review list', async ({ page }) => {
  await loadPage(page);
  await page.context().clearCookies();
  await page.reload();
  await expect(page.locator('#scopeToggle [data-scope="all"]')).toHaveClass(/active/);
  await expect(page.locator('#reviewConversation')).toBeHidden();
  const filter = page.getByRole('searchbox', { name: 'Filter changed files' });
  await filter.fill('server.go');
  await expect(page.locator('.tree-file')).toHaveCount(1);
  await expect(page.locator('.tree-file')).toContainText('server.go');
  await filter.fill('no-such-file');
  await expect(page.locator('#fileTreeBody')).toHaveText('No matching files.');
  await filter.fill('');
  const file = await goSection(page);
  await expect(file.locator('[data-line]').first()).toBeVisible();
});

test('single-file review follows navigation and returns to the full list', async ({ page }) => {
  await loadPage(page);
  await goSection(page);
  const toggle = page.getByRole('button', { name: 'Single file', exact: true });
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#filesContainer diffs-container')).toHaveCount(1);
  await expect(page.locator('.pierre-file-header')).toHaveAttribute('data-file-path', 'server.go');
  await revealFile(page, 'utils.go');
  await expect(page.locator('#filesContainer diffs-container')).toHaveCount(1);
  await expect(page.locator('.pierre-file-header')).toHaveAttribute('data-file-path', 'utils.go');
  const document = await switchToDocumentView(page);
  await expect(document.locator('.line-block').first()).toBeVisible();
  await revealFile(page, 'server.go');
  await revealFile(page, 'plan.md');
  await expect(document.locator('.line-block').first()).toBeVisible();
  await expect(page.locator('#filesContainer diffs-container')).toHaveCount(1);
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await expect.poll(() => page.locator('#filesContainer diffs-container').count()).toBeGreaterThan(1);
});

test('full source keeps old-side comments separate and path clicks do not collapse the file', async ({ page, request }) => {
  await expect(await request.post('/api/file/comments?path=server.go', { data: { start_line: 2, end_line: 2, side: 'old', body: 'Check the original line' } })).toBeOK();
  await loadPage(page);
  const file = await goSection(page);
  await file.locator('.file-header-name').click();
  await expect(file.locator('[data-line]').first()).toBeVisible();
  await file.getByRole('button', { name: 'File', exact: true }).click();
  await expect(file.locator('pre[data-file] [data-line]').first()).toBeVisible();
  await expect(file.locator('.outdated-diff-comments')).toContainText('Check the original line');
  await expect(file.locator('[data-line-annotation] .comment-card').filter({ hasText: 'Check the original line' })).toHaveCount(0);
  await file.getByRole('button', { name: 'Diff', exact: true }).click();
  await expect(file.locator('[data-line-type="change-addition"]').first()).toBeVisible();
});
