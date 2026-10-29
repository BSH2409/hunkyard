import { test, expect } from '@playwright/test';
import { spawn, execFileSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { readFixtureState } from './helpers';

test('three-dot CLI comparison shows the merge-base files and full commit IDs', async ({ page }) => {
  const fixture = readFixtureState(process.env.HUNKYARD_TEST_RANGE_PORT || '3128');
  const cwd = fixture.HUNKYARD_FIXTURE_DIR;
  const home = fixture.FAKE_HOME;
  const head = execFileSync('git', ['rev-parse', 'feat-c'], { cwd, encoding: 'utf8' }).trim();
  const daemon = spawn(fixture.HUNKYARD_BIN, ['_serve', '--no-open', '--port', '0', '--range', 'feat-d...feat-c'], {
    cwd, env: { ...process.env, HOME: home, USERPROFILE: home }, stdio: 'ignore',
  });
  try {
    let port = 0;
    await expect.poll(() => {
      const directory = path.join(home, '.hunkyard', 'sessions');
      for (const name of fs.readdirSync(directory)) {
        if (!name.endsWith('.json')) continue;
        const entry = JSON.parse(fs.readFileSync(path.join(directory, name), 'utf8'));
        if (entry.pid === daemon.pid) port = entry.port;
      }
      return port;
    }).toBeGreaterThan(0);
    const url = `http://127.0.0.1:${port}`;
    await expect.poll(async () => (await page.request.get(url + '/api/session')).status()).toBe(200);
    const session = await page.request.get(url + '/api/session').then(response => response.json());
    expect(session.focus).toMatchObject({
      range_mode: 'merge_base', base_sha: fixture.RANGE_HEAD,
      requested_base_sha: fixture.RANGE_HEAD_AFTER, head_sha: head,
    });
    await page.goto(url);
    await expect(page.locator('.loading')).toBeHidden();
    await expect(page.locator('.tree-file[data-tree-path="c.txt"]')).toBeVisible();
    await expect(page.locator('.tree-file[data-tree-path="b.txt"]')).toHaveCount(0);
  } finally {
    daemon.kill('SIGTERM');
    if (daemon.exitCode === null) await new Promise(resolve => daemon.once('exit', resolve));
  }
});
