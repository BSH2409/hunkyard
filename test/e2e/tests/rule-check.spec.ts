import { test, expect } from '@playwright/test';
import { mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import * as os from 'node:os';
import * as path from 'node:path';

// A standalone daemon whose fake home holds one active rule bound to the
// js_console_log diff check, so the shared fixtures' stores stay untouched.
let root: string, home: string, repo: string, env: NodeJS.ProcessEnv, daemon: ChildProcess, url: string, key: string;
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

test.skip(process.platform === 'win32', 'learned preferences are not supported on native Windows');

test.beforeAll(async ({ request }) => {
  root = mkdtempSync(path.join(os.tmpdir(), 'hunkyard-rules-'));
  home = path.join(root, 'home');
  repo = path.join(root, 'repo');
  mkdirSync(home);
  mkdirSync(repo);
  env = { ...process.env, HOME: home, USERPROFILE: home, CLAUDE_CONFIG_DIR: path.join(home, '.claude'), CODEX_HOME: path.join(home, '.codex'), HUNKYARD_PREFERENCES_HOME: path.join(home, '.agents'), HUNKYARD_NO_INTEGRATION_CHECK: '1', HUNKYARD_NO_UPDATE_CHECK: '1', HUNKYARD_OWNER_ID: `codex:rule-fixture:${root}` };
  const git = (...args: string[]) => execFileSync('git', ['-c', 'core.hooksPath=/dev/null', ...args], { cwd: repo, env, encoding: 'utf8' });
  const hy = (args: string[], input = '') => execFileSync(binary, args, { cwd: repo, env, encoding: 'utf8', input });
  git('init', '-q', '-b', 'main');
  git('config', 'user.name', 'Fixture');
  git('config', 'user.email', 'fixture@example.invalid');
  writeFileSync(path.join(repo, 'a.js'), 'function a() {\n  return 1;\n}\n');
  git('add', 'a.js');
  git('commit', '-qm', 'base');
  writeFileSync(path.join(repo, 'a.js'), 'function a() {\n  console.log("debug");\n  const x = 2;\n  return x;\n}\n');

  hy(['preferences', 'capture', '--explicit', '--directive', 'Remove console calls before committing.', '--scope', 'global', '--surface', 'frontend', '--json'], 'no console logs please');
  const found = JSON.parse(hy(['preferences', 'search', '--query', 'console', '--json']));
  const ruleID = found.rules[0].id;
  hy(['preferences', 'guard', 'add', '--kind', 'diff_check', '--rule', ruleID, '--pattern', 'js_console_log', '--json']);

  daemon = spawn(binary, ['_serve', '--no-open', '--host', '127.0.0.1', '--port', '0'], { cwd: repo, env, stdio: 'ignore' });
  let entry: { port: number; review_path: string } | null = null;
  await expect.poll(() => (entry = sessionEntry(daemon.pid)) && entry.port, { timeout: 15_000 }).toBeTruthy();
  url = `http://localhost:${entry!.port}`;
  key = path.basename(entry!.review_path);
  await expect.poll(async () => (await request.get(url + '/api/session')).status()).toBe(200);
});

test.afterAll(async () => {
  if (daemon?.exitCode === null && daemon?.signalCode === null) {
    const exited = once(daemon, 'exit');
    daemon.kill('SIGTERM');
    await exited;
  }
  if (root) rmSync(root, { recursive: true, force: true });
});

test('a learned rule flags an added line and the reviewer can dismiss it', async ({ page }) => {
  await page.goto(url);
  await expect(page.locator('.loading')).toBeHidden();
  const card = page.locator('.comment-card', { has: page.locator('.comment-source', { hasText: 'rule check' }) });
  await expect(card).toHaveCount(1, { timeout: 15_000 });
  await expect(card).toContainText('Likely violates: Remove console calls before committing.');
  await card.getByRole('button', { name: 'Dismiss' }).click();
  await expect.poll(async () => {
    const comments = await (await page.request.get(url + '/api/file/comments?path=a.js')).json();
    const rule = comments.find((c: { source?: string }) => c.source === 'rule');
    return rule && rule.verdict + ':' + rule.resolved;
  }).toBe('dismissed:true');
});

test('hunkyard annotate shows tool findings and uncovered lines', async ({ page }) => {
  const sarif = JSON.stringify({ runs: [{ tool: { driver: { name: 'eslint' } }, results: [{ ruleId: 'no-magic-numbers', message: { text: 'magic number 2' }, locations: [{ physicalLocation: { artifactLocation: { uri: 'a.js' }, region: { startLine: 3 } } }] }] }] });
  const out = execFileSync(binary, ['annotate', '--session', key, '--format', 'sarif', '-'], { cwd: repo, env, encoding: 'utf8', input: sarif });
  expect(out).toContain('eslint: added=1');
  execFileSync(binary, ['annotate', '--session', key, '--format', 'lcov', '-'], { cwd: repo, env, encoding: 'utf8', input: 'SF:a.js\nDA:2,0\nDA:3,1\nend_of_record\n' });

  await page.goto(url);
  await expect(page.locator('.loading')).toBeHidden();
  await expect(page.locator('.comment-source', { hasText: 'tool · eslint' })).toHaveCount(1, { timeout: 15_000 });
  await expect(page.locator('.file-header-coverage').first()).toHaveText('1 of 3 changed lines uncovered');
});
