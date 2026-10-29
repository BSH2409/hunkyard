import { test, expect } from '@playwright/test';
import { mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import * as os from 'node:os';
import * as path from 'node:path';
import { revealFile } from './helpers';

const reference = JSON.parse(readFileSync(path.join(__dirname, '../fixtures/github-review/reference.json'), 'utf8'));
let root: string, url: string, daemon: ChildProcess;

test.beforeAll(async () => {
  root = mkdtempSync(path.join(os.tmpdir(), 'hunkyard-github-'));
  const home = path.join(root, 'home'), repo = path.join(root, 'repo');
  mkdirSync(home); mkdirSync(repo);
  const env = { ...process.env, HOME: home, USERPROFILE: home, CODEX_HOME: path.join(home, '.codex'), CLAUDE_CONFIG_DIR: path.join(home, '.claude'), HUNKYARD_OWNER_ID: `codex:github-fixture:${root}`, HUNKYARD_NO_INTEGRATION_CHECK: '1' };
  const git = (...args: string[]) => execFileSync('git', ['-c', 'core.hooksPath=/dev/null', '-c', 'core.fsmonitor=false', ...args], { cwd: repo, env, encoding: 'utf8' }).trim();
  git('init', '-q'); git('config', 'user.name', 'Fixture'); git('config', 'user.email', 'fixture@example.invalid'); git('config', 'core.autocrlf', 'false');
  for (const side of ['base', 'head']) {
    writeFileSync(path.join(repo, 'README'), readFileSync(path.join(__dirname, `../fixtures/github-review/README.${side}`)));
    git('add', 'README'); git('commit', '-qm', `Pinned public ${side} contents`);
  }
  const binary = process.env.HUNKYARD_BIN || path.resolve(__dirname, '../../../dist/hunkyard');
  daemon = spawn(binary, ['_serve', '--no-open', '--port', '0', '--range', 'HEAD~1..HEAD'], { cwd: repo, env, stdio: 'ignore' });
  await expect.poll(() => {
    const sessions = path.join(home, '.hunkyard', 'sessions');
    try { for (const name of readdirSync(sessions)) { if (!name.endsWith('.json')) continue; const entry = JSON.parse(readFileSync(path.join(sessions, name), 'utf8')); if (entry.pid === daemon.pid) { url = `http://localhost:${entry.port}`; return entry.port; } } } catch {}
    return 0;
  }).toBeGreaterThan(0);
});

test.afterAll(async () => {
  if (daemon?.exitCode === null && daemon?.signalCode === null) { const exited = once(daemon, 'exit'); daemon.kill('SIGTERM'); await exited; }
  if (root) rmSync(root, { recursive: true, force: true });
});

for (const theme of ['light', 'dark']) {
  for (const diffMode of ['split', 'unified']) {
    test(`GitHub reference colors in ${theme} ${diffMode} review`, async ({ page }, info) => {
      await page.setViewportSize(reference.viewport);
      await page.context().addCookies([{ name: 'hunkyard-settings', value: encodeURIComponent(JSON.stringify({ theme, diffMode, diffScope: 'all' })), domain: 'localhost', path: '/' }]);
      await expect.poll(async () => (await page.request.get(url + '/api/session')).status()).toBe(200);
      await page.goto(url);
      await expect(page.locator('.loading')).toBeHidden();
      const item = await revealFile(page, 'README');
      const data = await page.request.get(url + '/api/file/diff?path=README').then(response => response.json());
      expect(data.hunks.flatMap((hunk: { Lines: { NoFinalNewline?: boolean }[] }) => hunk.Lines).filter((line: { NoFinalNewline?: boolean }) => line.NoFinalNewline)).toHaveLength(2);
      await expect(item.locator('[data-no-newline]').first()).toBeVisible();
      await expect(item.locator('[data-line][data-line-type="change-addition"]').first()).toBeVisible();
      const styles = reference.themes[theme];
      for (const [selector, expected] of [
        ['.pierre-file-header', styles['.file-header'].background],
        ['[data-line][data-line-type="change-addition"]', styles['.blob-code-addition'].background],
        ['[data-line][data-line-type="change-deletion"]', styles['.blob-code-deletion'].background],
        ['[data-column-number][data-line-type="change-addition"]', styles['.blob-num-addition'].background],
        ['[data-column-number][data-line-type="change-deletion"]', styles['.blob-num-deletion'].background],
      ]) {
        const target = item.locator(selector).first();
        await expect(target).toBeVisible();
        await expect.poll(async () => target.evaluate((element, expectedColor) => {
          const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1;
          const context = canvas.getContext('2d')!;
          const rgba = (color: string) => { context.clearRect(0, 0, 1, 1); context.fillStyle = color; context.fillRect(0, 0, 1, 1); return [...context.getImageData(0, 0, 1, 1).data]; };
          const actual = rgba(getComputedStyle(element).backgroundColor), expected = rgba(expectedColor);
          return Math.max(...actual.map((value, index) => Math.abs(value - expected[index])));
        }, expected), { message: `${theme} ${diffMode} ${selector} matches the captured GitHub color` }).toBeLessThanOrEqual(1);
      }
      await page.screenshot({ path: info.outputPath(`${theme}-${diffMode}.png`) });
    });
  }
}

test('binary changes remain reviewable as metadata and SVG remains source', async ({ page }) => {
  const repo = path.join(root, 'repo');
  const git = (...args: string[]) => execFileSync('git', ['-c', 'core.hooksPath=/dev/null', '-c', 'core.fsmonitor=false', ...args], { cwd: repo, encoding: 'utf8' }).trim();
  const base = git('rev-parse', 'HEAD');
  writeFileSync(path.join(repo, 'image.png'), Buffer.from([0, 1, 255]));
  writeFileSync(path.join(repo, 'drawing.svg'), '<svg><title>Review SVG as source</title></svg>\n');
  git('add', 'image.png', 'drawing.svg'); git('commit', '-qm', 'Local binary review fixture');
  const focus = await page.request.post(url + '/api/focus', { data: { kind: 'range', base_sha: base, head_sha: git('rev-parse', 'HEAD') } });
  await expect(focus).toBeOK();
  await page.goto(url);
  await expect(page.locator('.loading')).toBeHidden();
  const image = await revealFile(page, 'image.png');
  await expect(image).toContainText('Binary file changed.');
  expect(await page.request.get(url + '/api/file?path=image.png').then(r => r.json())).toMatchObject({ file_type: 'binary', content: '' });
  const svg = await revealFile(page, 'drawing.svg');
  await expect(svg).toContainText('Review SVG as source');
  await expect(svg.locator('svg title')).toHaveCount(0);
  const publication = await page.request.post(url + '/api/finish', { data: { request_id: 'binary-fixture', disposition: 'comment' } });
  await expect(publication).toBeOK();
  expect((await publication.json()).batch.snapshot_id).toMatch(/^snapshot_/);
});
