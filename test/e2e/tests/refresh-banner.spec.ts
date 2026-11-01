import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import { clearAllComments, countRequests, loadPage, sessionCwd } from './helpers';

const PROBE = 'banner-probe.txt';

test.beforeEach(async ({ request }) => {
  await clearAllComments(request);
});

test('changes outside a round show a Refresh banner instead of redrawing', async ({ page, browser, request }) => {
  const dir = await sessionCwd(request);
  const git = (...args: string[]) => execFileSync('git', args, { cwd: dir });
  await loadPage(page);
  const other = await browser.newPage();
  await loadPage(other);
  const banner = page.locator('#reviewUpdatesBanner');
  await expect(banner).toBeHidden();
  const reloads = countRequests(page, '/api/session');
  try {
    fs.writeFileSync(path.join(dir, PROBE), 'staged outside the round\n');
    git('add', PROBE); // moves the index, which the watcher probes every second
    await expect(banner).toBeVisible({ timeout: 10_000 });
    await expect(banner).toContainText('This review has updates');
    await expect(other.locator('#reviewUpdatesBanner')).toBeVisible();
    // Nothing redrew. The watcher sends its other events on the same tick, so
    // wait past the next tick (1 s) before checking that nothing reloaded.
    await page.waitForTimeout(1500); // a negative is only observable over time
    expect(reloads.n).toBe(0);
    await expect(page.locator(`.tree-file[data-tree-path="${PROBE}"]`)).toHaveCount(0);
    await page.locator('#reviewUpdatesRefresh').click();
    await expect(banner).toBeHidden();
    await expect(page.locator(`.tree-file[data-tree-path="${PROBE}"]`)).toHaveCount(1);
    // Positive control: the counter does see the reload Refresh makes.
    expect(reloads.n).toBeGreaterThan(0);
    // A fresh load does not show the banner once refreshed.
    await page.reload();
    await expect(page.locator('.loading')).toBeHidden();
    await expect(banner).toBeHidden();
  } finally {
    await other.close();
    git('rm', '--cached', '-q', '-f', PROBE);
    fs.rmSync(path.join(dir, PROBE), { force: true });
    // Let the watcher see the cleanup before refreshing, so the next test
    // does not inherit a stale review.
    await expect.poll(async () => (await (await request.get('/api/session')).json()).stale === true, { timeout: 10_000 }).toBe(true);
    await request.post('/api/refresh');
  }
});
