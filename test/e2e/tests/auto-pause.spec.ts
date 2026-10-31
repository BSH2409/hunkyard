import { test, expect } from '@playwright/test';
import { existsSync, mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import * as os from 'node:os';
import * as path from 'node:path';

// A standalone daemon with a 3-second idle pause (test mode only), so the
// shared fixtures' daemons are never paused.
let root: string, home: string, repo: string, env: NodeJS.ProcessEnv, daemon: ChildProcess;
const binary = process.env.HUNKYARD_BIN || path.resolve(__dirname, '../../../hunkyard');

function sessionEntry(pid: number | undefined): { port: number; review_path: string } | null {
  const sessions = path.join(home, '.hunkyard', 'sessions');
  try {
    for (const name of readdirSync(sessions)) {
      if (!name.endsWith('.json')) continue;
      const entry = JSON.parse(readFileSync(path.join(sessions, name), 'utf8'));
      if (entry.pid === pid) return entry;
    }
  } catch { /* not written yet */ }
  return null;
}

test.beforeAll(() => {
  root = mkdtempSync(path.join(os.tmpdir(), 'hunkyard-pause-'));
  home = path.join(root, 'home');
  repo = path.join(root, 'repo');
  mkdirSync(home);
  mkdirSync(repo);
  env = { ...process.env, HOME: home, USERPROFILE: home, HUNKYARD_NO_INTEGRATION_CHECK: '1', HUNKYARD_NO_UPDATE_CHECK: '1', HUNKYARD_TEST_MODE: '1', HUNKYARD_IDLE_PAUSE_SECONDS: '3', HUNKYARD_OWNER_ID: `codex:pause-fixture:${root}` };
  const git = (...args: string[]) => execFileSync('git', ['-c', 'core.hooksPath=/dev/null', ...args], { cwd: repo, env, encoding: 'utf8' });
  git('init', '-q', '-b', 'main');
  git('config', 'user.name', 'Fixture');
  git('config', 'user.email', 'fixture@example.invalid');
  writeFileSync(path.join(repo, 'a.go'), 'package a\n');
  git('add', 'a.go');
  git('commit', '-qm', 'base');
  writeFileSync(path.join(repo, 'a.go'), 'package a\n\nvar X = 1\n');
});

test.afterAll(async () => {
  if (daemon?.exitCode === null && daemon?.signalCode === null) {
    const exited = once(daemon, 'exit');
    daemon.kill('SIGTERM');
    await exited;
  }
  if (root) rmSync(root, { recursive: true, force: true });
});

test('an idle review pauses, frees its port and tells the open tab', async ({ page }) => {
  daemon = spawn(binary, ['_serve', '--no-open', '--host', '127.0.0.1', '--port', '0'], { cwd: repo, env, stdio: 'ignore' });
  const exited = once(daemon, 'exit');
  let entry: { port: number; review_path: string } | null = null;
  await expect.poll(() => (entry = sessionEntry(daemon.pid)) && entry.port, { timeout: 15_000 }).toBeTruthy();
  const url = `http://localhost:${entry!.port}`;
  await expect.poll(async () => (await page.request.get(url + '/api/session')).status()).toBe(200);
  // A comment made before the pause must survive it.
  await expect(await page.request.post(url + '/api/file/comments?path=a.go', { data: { start_line: 3, end_line: 3, body: 'Survives the pause' } })).toBeOK();
  await page.goto(url);
  await expect(page.locator('.loading')).toBeHidden();

  const overlay = page.locator('#pausedOverlay');
  await expect(overlay).toBeVisible({ timeout: 15_000 });
  await expect(overlay).toContainText('Review paused');
  await expect(overlay).toContainText('Ask your agent to resume this review.');
  await expect(overlay.locator('.paused-command code')).toHaveText(/^hunkyard --session [0-9a-f]+$/);
  // The pause replaces "Session complete": the review is resumable.
  await expect(page.locator('.disconnected-banner')).toHaveCount(0);

  await exited;
  expect(existsSync(path.join(entry!.review_path, 'paused.json'))).toBe(true);
  await expect.poll(async () => page.request.get(url + '/api/health').then(() => 'up', () => 'down')).toBe('down');

  // Resume as the agent would; the open tab reconnects by itself on the same port.
  const key = path.basename(entry!.review_path);
  const client = spawn(binary, ['--session', key, '--no-open'], { cwd: repo, env: { ...env, HUNKYARD_IDLE_PAUSE_SECONDS: '600' }, stdio: 'ignore' });
  try {
    await expect(overlay).toBeHidden({ timeout: 20_000 });
    await expect(page.locator('.loading')).toBeHidden();
    expect(new URL(page.url()).port).toBe(String(entry!.port));
    const comments = await (await page.request.get(url + '/api/file/comments?path=a.go')).json();
    expect(comments.map((c: { body: string }) => c.body)).toContain('Survives the pause');
    expect(existsSync(path.join(entry!.review_path, 'paused.json'))).toBe(false);
  } finally {
    client.kill('SIGTERM');
    const resumed = sessionEntry(undefined) ?? readdirSync(path.join(home, '.hunkyard', 'sessions')).filter(n => n.endsWith('.json')).map(n => JSON.parse(readFileSync(path.join(home, '.hunkyard', 'sessions', n), 'utf8')))[0];
    if (resumed?.pid) try { process.kill(resumed.pid, 'SIGTERM'); } catch { /* already gone */ }
  }
});
