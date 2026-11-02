import { test, expect } from '@playwright/test';
import { mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import * as os from 'node:os';
import * as path from 'node:path';
import { revealFile, treePaths } from './helpers';

// A standalone daemon on a small repo whose changed api.go is called from
// routes.go and tested by api_test.go, next to a sensitive auth file.
let root: string, home: string, daemon: ChildProcess, url: string;
const binary = process.env.HUNKYARD_BIN || path.resolve(__dirname, '../../../hunkyard');

test.beforeAll(async ({ request }) => {
  root = mkdtempSync(path.join(os.tmpdir(), 'hunkyard-nav-'));
  home = path.join(root, 'home');
  const repo = path.join(root, 'repo');
  mkdirSync(home); mkdirSync(path.join(repo, 'auth'), { recursive: true });
  const env = { ...process.env, HOME: home, USERPROFILE: home, CLAUDE_CONFIG_DIR: path.join(home, '.claude'), CODEX_HOME: path.join(home, '.codex'), HUNKYARD_PREFERENCES_HOME: path.join(home, '.agents'), HUNKYARD_NO_INTEGRATION_CHECK: '1', HUNKYARD_NO_UPDATE_CHECK: '1', HUNKYARD_OWNER_ID: `codex:nav-fixture:${root}` };
  const git = (...args: string[]) => execFileSync('git', ['-c', 'core.hooksPath=/dev/null', ...args], { cwd: repo, env, encoding: 'utf8' });
  git('init', '-q', '-b', 'main');
  git('config', 'user.name', 'Fixture');
  git('config', 'user.email', 'fixture@example.invalid');
  writeFileSync(path.join(repo, 'api.go'), 'package a\n\nfunc ServeLogin() {}\n');
  writeFileSync(path.join(repo, 'routes.go'), 'package a\n\nvar loginRoute = ServeLogin\n');
  writeFileSync(path.join(repo, 'api_test.go'), 'package a\n\nimport "testing"\n\nfunc TestServeLogin(t *testing.T) { ServeLogin() }\n');
  writeFileSync(path.join(repo, 'auth', 'session.go'), 'package auth\n');
  writeFileSync(path.join(repo, 'NOTES.md'), '# Notes\n');
  git('add', '.');
  git('commit', '-qm', 'base');
  writeFileSync(path.join(repo, 'api.go'), 'package a\n\nfunc ServeLogin() {\n\tprintln("login")\n}\n');
  writeFileSync(path.join(repo, 'auth', 'session.go'), 'package auth\n\nvar Secret = 1\n');
  writeFileSync(path.join(repo, 'NOTES.md'), '# Notes\n\nOne line.\n');
  daemon = spawn(binary, ['_serve', '--no-open', '--host', '127.0.0.1', '--port', '0'], { cwd: repo, env, stdio: 'ignore' });
  await expect.poll(() => {
    try {
      for (const name of readdirSync(path.join(home, '.hunkyard', 'sessions'))) {
        if (!name.endsWith('.json')) continue;
        const entry = JSON.parse(readFileSync(path.join(home, '.hunkyard', 'sessions', name), 'utf8'));
        if (entry.pid === daemon.pid) { url = `http://localhost:${entry.port}`; return entry.port; }
      }
    } catch { /* not written yet */ }
    return 0;
  }, { timeout: 15_000 }).toBeGreaterThan(0);
  await expect.poll(async () => (await request.get(url + '/api/session')).status()).toBe(200);
});

test.afterAll(async () => {
  if (daemon?.exitCode === null && daemon?.signalCode === null) { const exited = once(daemon, 'exit'); daemon.kill('SIGTERM'); await exited; }
  if (root) rmSync(root, { recursive: true, force: true });
});

test('the impact pill lists callers and tests of what a file changes', async ({ page }) => {
  await page.goto(url);
  await expect(page.locator('.loading')).toBeHidden();
  const item = await revealFile(page, 'api.go');
  const pill = page.locator('.pierre-file-header[data-file-path="api.go"] .file-header-impact');
  await expect(pill).toHaveText('1 caller · 1 test');
  await expect(pill).toHaveAttribute('title', /1 caller · 1 test · 0 tests changed/);
  await pill.click();
  const panel = item.locator('.impact-panel');
  await expect(panel).toBeVisible();
  await expect(panel.locator('.impact-ref', { hasText: 'routes.go:3' })).toBeVisible();
  await expect(panel).toContainText('api_test.go');
  await expect(pill).toHaveAttribute('aria-pressed', 'true');
});

test('the overview orders files by risk and the tree follows', async ({ page }) => {
  await page.goto(url);
  await expect(page.locator('.loading')).toBeHidden();
  const pathOrder = await treePaths(page);
  expect(pathOrder).toEqual(['auth/session.go', 'NOTES.md', 'api.go']);
  await page.locator('#prToggle').click();
  const overview = page.locator('#prPanel');
  await expect(overview).toContainText('Review overview');
  await expect(overview.locator('.overview-commit-list')).toContainText('base');
  await overview.getByRole('button', { name: 'By risk' }).click();
  await expect.poll(() => treePaths(page)).toEqual(['auth/session.go', 'api.go', 'NOTES.md']);
  await expect(overview.locator('.overview-risk-list li').first()).toContainText('sensitive path');
  const listOrder = await page.locator('#filesContainer .pierre-file-header[data-file-path]').evaluateAll(els => els.map(el => (el as HTMLElement).dataset.filePath));
  expect(listOrder).toEqual(['auth/session.go', 'api.go', 'NOTES.md']);
});
