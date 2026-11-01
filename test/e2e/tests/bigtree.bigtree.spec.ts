import { test, expect, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { clearAllComments, countRequests, loadPage, realCommitItems } from './helpers';
import { stateFilePath } from './state-file';

// A 5,000-file tree with 300 untracked and 30,000 ignored files, reviewed
// through selected code paths (setup-fixtures-bigtree.sh). Wall-clock limits
// here are generous ceilings, not budgets: the git-call budgets live in the
// Go tests.

const PORT = process.env.HUNKYARD_TEST_BIGTREE_PORT || '3138';
// Shared CI runners are several times slower than a laptop; the ceiling is
// there to catch a regression to full-tree walks, not to time the runner.
const CEILING_MS = process.env.CI ? 15_000 : 5_000;

function fixtureValue(key: string): string {
  const line = readFileSync(stateFilePath(PORT), 'utf8').split('\n').find(l => l.startsWith(key + '='));
  if (!line) throw new Error(`${key} missing from the bigtree state file`);
  return line.slice(key.length + 1);
}

const treeFiles = (page: Page) => page.locator('.tree-file[data-tree-path]');

async function chooseBase(page: Page, pick: (page: Page) => Promise<void>, label: RegExp | string) {
  await page.locator('#baseBranchBtn').click();
  await pick(page);
  await expect(page.locator('#baseBranchLabel')).toHaveText(label);
  await expect(page.locator('#filesContainer')).not.toHaveAttribute('aria-busy', 'true');
}

test.beforeEach(async ({ request }) => {
  await clearAllComments(request);
});

test('the review opens within 5 seconds (15 on CI)', async ({ page }) => {
  const start = Date.now();
  await loadPage(page);
  await expect(treeFiles(page).first()).toBeVisible({ timeout: CEILING_MS });
  test.info().annotations.push({ type: 'open-ms', description: String(Date.now() - start) });
});

test('Working changes lists the untracked package within 5 seconds (15 on CI)', async ({ page }) => {
  await loadPage(page);
  await page.click('#commitDropdownBtn');
  const start = Date.now();
  await page.locator('#commitDropdownList .commit-picker-item.is-virtual').click();
  const untracked = page.locator('.tree-file[data-tree-path^="newpkg/"]').first();
  await expect(untracked).toBeVisible({ timeout: CEILING_MS });
  test.info().annotations.push({ type: 'working-changes-ms', description: String(Date.now() - start) });
  const path = await untracked.getAttribute('data-tree-path');
  const diff = await (await page.request.get(`/api/file/diff?path=${encodeURIComponent(path!)}&commit=__hunkyard_virtual_working_tree__`)).json();
  const lines = (diff.hunks ?? diff).flatMap((h: { Lines?: { Type: string }[]; lines?: { type: string }[] }) => h.Lines ?? h.lines ?? []);
  expect(lines.some((l: { Type?: string; type?: string }) => (l.Type ?? l.type) === 'add')).toBe(true);
});

test('a base change and its revert each load the commit list once', async ({ page, request }) => {
  const original = (await (await request.get('/api/session')).json()).base_branch_name;
  try {
    await loadPage(page);
    // The base picker's own ?scope=compare list is not the review's commit list.
    const commitLists = countRequests(page, '/api/commits', '');
    // The picker takes a SHA, not main~2, as a typed revision.
    const base2 = fixtureValue('BASE2_SHA');
    await chooseBase(page, async p => {
      await p.locator('#baseBranchSearch').fill(base2);
      await p.locator('#baseBranchSearch').press('Enter');
    }, base2.slice(0, 7));
    await expect.poll(() => commitLists.n).toBe(1);
    await chooseBase(page, async p => {
      await p.locator(`.base-branch-item[data-branch="${original}"]`).click();
    }, original);
    await expect.poll(() => commitLists.n).toBe(2);
    await page.waitForTimeout(1000); // a duplicate reload is only observable over time
    expect(commitLists.n).toBe(2);
  } finally {
    await request.post('/api/base-branch', { data: { ref: original } });
  }
});

test('picking two commits quickly shows the second one', async ({ page, request }) => {
  await loadPage(page);
  await page.click('#commitDropdownBtn');
  const rows = realCommitItems(page);
  await expect(rows).toHaveCount(3);
  const second = await rows.nth(1).getAttribute('data-commit');
  await rows.nth(0).click();
  await rows.nth(1).click(); // supersedes the first pick before it lands
  const expected = (await (await request.get(`/api/session?commit=${second}`)).json()).files.map((f: { path: string }) => f.path).sort();
  await expect.poll(async () => (await treeFiles(page).evaluateAll(els => els.map(e => e.getAttribute('data-tree-path') || '')))
    .filter(Boolean).sort()).toEqual(expected);
});
