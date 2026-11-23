import { test, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { clearAllComments, loadPage } from './helpers';
import { ensureRangeFocus, rangeFixture } from './range-helpers';

test.beforeEach(async ({ request }) => {
  await clearAllComments(request);
  await ensureRangeFocus(request);
});

test('a pinned review explicitly switches to the working tree to show staged changes', async ({ page, request }) => {
  const dir = rangeFixture().fixtureDir;
  const name = 'staged-review-probe.go';
  try {
    fs.writeFileSync(path.join(dir, name), 'package probe\n');
    execFileSync('git', ['add', name], { cwd: dir });
    await loadPage(page);
    const since = page.getByRole('button', { name: 'Since last review' });
    await expect(since).toBeDisabled();
    await expect(since).toHaveAccessibleDescription('Switch to the working tree.');
    await expect(page.locator('#reviewUpdatesBanner')).toContainText('Pinned to commits');
    await expect(page.locator(`.tree-file[data-tree-path="${name}"]`)).toHaveCount(0);
    await page.getByRole('button', { name: 'Review working tree', exact: true }).click();
    await expect(page.locator(`.tree-file[data-tree-path="${name}"]`)).toHaveCount(1);
    await expect.poll(async () => (await (await request.get('/api/session')).json()).focus.kind).toBe('working_tree');
    await expect(page.locator('#reviewUpdatesBanner')).toBeHidden();
    await expect(since).toBeEnabled();
    await since.click();
    await expect(since).toHaveAttribute('aria-pressed', 'true');
    await page.locator('#resumePrPill').click();
    await expect(since).toBeDisabled();
    await expect(page.getByRole('button', { name: 'Full diff' })).toHaveAttribute('aria-pressed', 'true');
  } finally {
    execFileSync('git', ['rm', '--cached', '-f', name], { cwd: dir });
    fs.rmSync(path.join(dir, name), { force: true });
    await ensureRangeFocus(request);
  }
});
