import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage, switchToDocumentView } from './helpers';

// One load covers the git-mode page chrome. File tree status icons and stats
// are covered by file-tree.spec.ts.
test.describe('Page Loading', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllComments(request);
  });

  test('git-mode page loads with branch context, diff toggle, no PR button, and no mermaid', async ({ page }) => {
    // Pluggable renderers load their libraries on first use: a
    // review without diagrams keeps today's page weight.
    const requested: string[] = [];
    page.on('request', req => requested.push(req.url()));
    await loadPage(page);

    // Loading disappears, file sections appear.
    await expect(page.locator('.pierre-file-header').first()).toBeVisible();
    await expect(page.locator('diffs-container [data-line]').first()).toBeVisible();

    // Branch name in the header and the document title.
    await expect(page.locator('#branchContext')).toBeVisible();
    await expect(page.locator('#branchName')).toHaveText('feat/add-auth');
    await expect(page).toHaveTitle(/Hunkyard — feat\/add-auth/);

    await expect(page.locator('#diffModeToggle')).toBeVisible();
    // No PR exists.
    await expect(page.locator('.pr-toggle-btn')).not.toBeVisible();

    const doc = await switchToDocumentView(page);
    await expect(doc.locator('.line-block').first()).toBeVisible();
    await expect(page.locator('.hunkyard-render')).toHaveCount(0);
    expect(requested.some(u => u.endsWith('/app.js'))).toBe(true);
    expect(requested.filter(u => u.includes('mermaid.min.js'))).toEqual([]);
  });
});
