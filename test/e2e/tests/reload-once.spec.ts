import { test, expect, type Page } from '@playwright/test';
import { clearAllComments, countRequests, loadPage, realCommitItems } from './helpers';

// Record whether the file list was ever blanked to "Loading..." or marked busy.
async function watchFileList(page: Page) {
  await page.evaluate(() => {
    const w = window as unknown as { __sawLoading: boolean; __sawBusy: boolean };
    w.__sawLoading = false;
    w.__sawBusy = false;
    const container = document.getElementById('filesContainer')!;
    new MutationObserver(() => {
      if (container.querySelector(':scope > .loading')) w.__sawLoading = true;
      if (container.getAttribute('aria-busy') === 'true') w.__sawBusy = true;
    }).observe(container, { childList: true, attributes: true, attributeFilter: ['aria-busy'] });
  });
}

test.beforeEach(async ({ request }) => {
  await clearAllComments(request);
});

test('a base change loads the review and the commit list once, without blanking the list', async ({ page, request }) => {
  const original = await (await request.get('/api/session')).json();
  const commits = (await (await request.get('/api/commits')).json()).filter((c: { virtual?: boolean }) => !c.virtual);
  try {
    await loadPage(page);
    await expect(page.locator('#commitDropdown')).toBeVisible();
    await watchFileList(page);
    const sessions = countRequests(page, '/api/session');
    // The base picker's own list (?scope=compare) is not the review's commit list.
    const commitLists = countRequests(page, '/api/commits', '');
    await page.locator('#baseBranchBtn').click();
    await page.locator('.compare-target-tab[data-tab="commits"]').click();
    await page.locator(`.base-branch-item[data-branch="${commits[1].sha}"]`).click();
    await expect(page.locator('#baseBranchLabel')).toHaveText(commits[1].short_sha);
    await expect(page.locator('#filesContainer')).not.toHaveAttribute('aria-busy', 'true');
    // Give a second, duplicate reload the time it used to take to start.
    await expect.poll(() => sessions.n, { timeout: 3000 }).toBeGreaterThan(0);
    await page.waitForTimeout(1000); // a negative is only observable over time
    expect(sessions.n).toBe(1);
    expect(commitLists.n).toBe(1);
    const flags = await page.evaluate(() => {
      const w = window as unknown as { __sawLoading: boolean; __sawBusy: boolean };
      return { sawLoading: w.__sawLoading, sawBusy: w.__sawBusy };
    });
    expect(flags.sawLoading).toBe(false);
    expect(flags.sawBusy).toBe(true);
  } finally {
    await request.post('/api/base-branch', { data: { ref: original.base_branch_name } });
  }
});

test('picking two commits quickly shows the second one', async ({ page, request }) => {
  await loadPage(page);
  await page.click('#commitDropdownBtn');
  const rows = realCommitItems(page);
  await expect(rows).toHaveCount(2);
  const second = await rows.nth(1).getAttribute('data-commit');
  await rows.nth(0).click();
  await rows.nth(1).click(); // supersedes the first pick before it lands
  const expected = (await (await request.get(`/api/session?commit=${second}`)).json()).files.map((f: { path: string }) => f.path).sort();
  await expect.poll(async () => (await page.locator('.tree-file').evaluateAll(els => els.map(e => e.getAttribute('data-tree-path') || '')))
    .filter(Boolean).sort()).toEqual(expected);
});
