import { test, expect } from '@playwright/test';
import { existsSync, mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import * as os from 'node:os';
import * as path from 'node:path';

// Two standalone daemons in one fake home: one pauses after 3 seconds (test
// mode), the other serves the inbox page that resumes it.
let root: string, home: string, env: NodeJS.ProcessEnv;
const daemons: ChildProcess[] = [];
const binary = process.env.HUNKYARD_BIN || path.resolve(__dirname, '../../../hunkyard');

function repo(name: string): string {
  const dir = path.join(root, name);
  mkdirSync(dir);
  const git = (...args: string[]) => execFileSync('git', ['-c', 'core.hooksPath=/dev/null', ...args], { cwd: dir, env, encoding: 'utf8' });
  git('init', '-q', '-b', 'main');
  git('config', 'user.name', 'Fixture');
  git('config', 'user.email', 'fixture@example.invalid');
  writeFileSync(path.join(dir, 'a.go'), 'package a\n');
  git('add', 'a.go');
  git('commit', '-qm', 'base');
  writeFileSync(path.join(dir, 'a.go'), 'package a\n\nvar X = 1\n');
  return dir;
}

function sessionEntry(pid: number | undefined): { port: number; review_path: string } | null {
  try {
    for (const name of readdirSync(path.join(home, '.hunkyard', 'sessions'))) {
      if (!name.endsWith('.json')) continue;
      const entry = JSON.parse(readFileSync(path.join(home, '.hunkyard', 'sessions', name), 'utf8'));
      if (entry.pid === pid) return entry;
    }
  } catch { /* not written yet */ }
  return null;
}

async function startDaemon(cwd: string, extraEnv: NodeJS.ProcessEnv) {
  const child = spawn(binary, ['_serve', '--no-open', '--host', '127.0.0.1', '--port', '0'], { cwd, env: { ...env, ...extraEnv }, stdio: 'ignore' });
  daemons.push(child);
  let entry: { port: number; review_path: string } | null = null;
  await expect.poll(() => (entry = sessionEntry(child.pid)) && entry.port, { timeout: 15_000 }).toBeTruthy();
  return { child, entry: entry! };
}

test.beforeAll(() => {
  root = mkdtempSync(path.join(os.tmpdir(), 'hunkyard-inbox-'));
  home = path.join(root, 'home');
  mkdirSync(home);
  env = { ...process.env, HOME: home, USERPROFILE: home, CLAUDE_CONFIG_DIR: path.join(home, '.claude'), CODEX_HOME: path.join(home, '.codex'), HUNKYARD_PREFERENCES_HOME: path.join(home, '.agents'), HUNKYARD_NO_INTEGRATION_CHECK: '1', HUNKYARD_NO_UPDATE_CHECK: '1', HUNKYARD_TEST_MODE: '1', HUNKYARD_OWNER_ID: `codex:inbox-fixture:${root}` };
});

test.afterAll(async () => {
  for (const child of daemons) {
    if (child.exitCode === null && child.signalCode === null) { const exited = once(child, 'exit'); child.kill('SIGTERM'); await exited; }
  }
  // A resumed daemon is not our child: stop every review the fake home knows.
  try { execFileSync(binary, ['stop', '--all'], { env, stdio: 'ignore' }); } catch { /* none left */ }
  if (root) rmSync(root, { recursive: true, force: true });
});

test('the inbox lists running and paused reviews and resumes a paused one', async ({ page, request }) => {
  const pausing = await startDaemon(repo('pausing'), { HUNKYARD_IDLE_PAUSE_SECONDS: '3' });
  await expect.poll(() => existsSync(path.join(pausing.entry.review_path, 'paused.json')), { timeout: 20_000 }).toBe(true);
  const host = await startDaemon(repo('host'), { HUNKYARD_IDLE_PAUSE_SECONDS: '600' });
  const url = `http://localhost:${host.entry.port}`;
  await expect.poll(async () => (await request.get(url + '/api/session')).status()).toBe(200);

  await page.goto(url + '/inbox');
  const rows = page.locator('.inbox-row');
  await expect(rows).toHaveCount(2);
  await expect(page.locator('.inbox-row--paused')).toContainText('pausing');
  await expect(page.locator('.inbox-row--waiting_for_reviewer')).toContainText('host');
  await expect(page.locator('#inboxSummary')).toHaveText('1 waiting for you · 1 paused');

  await page.locator('.inbox-row--paused').getByRole('button', { name: 'Resume' }).click();
  await expect(page.locator('.inbox-row--paused')).toHaveCount(0, { timeout: 20_000 });
  await expect(page.locator('.inbox-row--waiting_for_reviewer')).toHaveCount(2);
  expect(existsSync(path.join(pausing.entry.review_path, 'paused.json'))).toBe(false);
});
