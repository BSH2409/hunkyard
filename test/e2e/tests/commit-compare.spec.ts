import { test, expect, type Locator, type Page } from '@playwright/test';
import { clearAllComments, loadPage } from './helpers';

async function openCompareCommits(page: Page) {
  await page.locator('#baseBranchBtn').click();
  await page.locator('.compare-target-tab[data-tab="commits"]').click();
}

// The whole message shows, unclipped. Pickers open with a transition, so the
// text can match while the panel still measures 0x0: wait for it to be
// visible and retry the measurement until the layout settles.
async function expectMessageFits(message: Locator) {
  await expect(message).toBeVisible();
  await expect.poll(() => message.evaluate(el =>
    el.clientWidth > 0 && el.scrollWidth <= el.clientWidth && el.scrollHeight <= el.clientHeight)).toBe(true);
}

test.beforeEach(async ({ request }) => {
  await clearAllComments(request);
});

test('compare commits retain selected and older choices after changing the base and reloading', async ({ page, request }) => {
  const original = await (await request.get('/api/session')).json();
  const commits = (await (await request.get('/api/commits')).json()).filter((c: { virtual?: boolean }) => !c.virtual);
  expect(commits).toHaveLength(2);
  const [newer, older] = commits;
  try {
    await loadPage(page);
    await openCompareCommits(page);
    await expect(page.locator('.compare-commit-item')).toHaveCount(2);
    const selected = page.locator(`.base-branch-item[data-branch="${newer.sha}"]`);
    await selected.click();
    await expect(page.locator('#baseBranchLabel')).toHaveText(newer.short_sha);

    await page.reload();
    await expect(page.locator('.loading')).toBeHidden();
    await openCompareCommits(page);
    await expect(selected).toBeVisible();
    await expect(selected).toHaveClass(/active/);
    await expect(page.locator('.compare-commit-item')).toHaveCount(2);
    await expect(selected).toHaveAttribute('aria-current', 'true');
    await expect(selected).toContainText(newer.message);
    await expect(selected.locator('.commit-picker-item-time')).toContainText(/ago|just now/);
    await page.locator(`.base-branch-item[data-branch="${older.sha}"]`).click();
    await expect(page.locator('#baseBranchLabel')).toHaveText(older.short_sha);
    await expect.poll(async () => (await (await request.get('/api/session')).json()).base_ref).toBe(older.sha);
  } finally {
    const restored = await request.post('/api/base-branch', { data: { ref: original.base_branch_name } });
    expect(restored.ok()).toBeTruthy();
  }
});

test('both commit pickers show full local timestamps for commits at least 24 hours old', async ({ page, request }) => {
  const commits = (await (await request.get('/api/commits')).json()).filter((c: { virtual?: boolean }) => !c.virtual);
  const commit = commits[0];
  await page.clock.setFixedTime(new Date(Date.now() + 2 * 86400000));
  await loadPage(page);
  const fullTime = await page.evaluate(date => new Date(date).toLocaleString([], { dateStyle: 'medium', timeStyle: 'long' }), commit.date);
  await page.locator('#commitDropdownBtn').click();
  const sidebarCommit = page.locator(`.commit-picker-item[data-commit="${commit.sha}"]`);
  await expect(sidebarCommit.locator('time')).toHaveText(fullTime);
  await expect(sidebarCommit.locator('.commit-picker-item-msg')).toHaveText(commit.message);
  await expectMessageFits(sidebarCommit.locator('.commit-picker-item-msg'));
  await page.keyboard.press('Escape');
  await openCompareCommits(page);
  const headerCommit = page.locator(`.compare-commit-item[data-branch="${commit.sha}"]`);
  await expect(headerCommit.locator('time')).toHaveText(fullTime);
  await expect(headerCommit.locator('.commit-picker-item-msg')).toHaveText(commit.message);
  await expectMessageFits(headerCommit.locator('.commit-picker-item-msg'));
});
