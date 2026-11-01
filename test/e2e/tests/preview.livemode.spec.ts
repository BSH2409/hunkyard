import { test, expect } from '@playwright/test';
import { mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import * as os from 'node:os';
import * as path from 'node:path';
import { enterPinMode } from './livemode-helpers';

// `hunkyard preview page.html` serves a local HTML file through the same pin
// UI as live mode; pins there capture screenshots too. This spec runs its own
// daemon so the shared live fixture is untouched.
let root: string, home: string, env: NodeJS.ProcessEnv, daemon: ChildProcess, url: string;
const binary = process.env.HUNKYARD_BIN || path.resolve(__dirname, '../../../hunkyard');

const PAGE = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>preview fixture</title>
<style>body { margin: 0; font: 16px system-ui, sans-serif; background: oklch(96% 0.02 90); }
#cta { margin: 40px; padding: 12px 20px; border: 0; background: color-mix(in srgb, #2f6fde 80%, black); color: white; }</style>
</head><body><h1 id="heading">Preview fixture</h1><button id="cta">Start trial</button></body></html>`;

test.beforeAll(async () => {
  root = mkdtempSync(path.join(os.tmpdir(), 'hunkyard-preview-'));
  home = path.join(root, 'home');
  const site = path.join(root, 'site');
  mkdirSync(home);
  mkdirSync(site);
  writeFileSync(path.join(site, 'index.html'), PAGE);
  env = { ...process.env, HOME: home, USERPROFILE: home, CLAUDE_CONFIG_DIR: path.join(home, '.claude'), CODEX_HOME: path.join(home, '.codex'), HUNKYARD_PREFERENCES_HOME: path.join(home, '.agents'), HUNKYARD_NO_INTEGRATION_CHECK: '1', HUNKYARD_NO_UPDATE_CHECK: '1', HUNKYARD_OWNER_ID: `codex:preview-fixture:${root}` };
  daemon = spawn(binary, ['_serve', '--no-open', '--host', '127.0.0.1', '--port', '0', '--preview-file', path.join(site, 'index.html')], { cwd: site, env, stdio: 'ignore' });
  let port = 0;
  await expect.poll(() => {
    try {
      const dir = path.join(home, '.hunkyard', 'sessions');
      for (const name of readdirSync(dir)) {
        if (!name.endsWith('.json')) continue;
        const entry = JSON.parse(readFileSync(path.join(dir, name), 'utf8'));
        if (entry.pid === daemon.pid) port = entry.port;
      }
    } catch { /* not written yet */ }
    return port;
  }, { timeout: 15_000 }).toBeGreaterThan(0);
  url = `http://localhost:${port}`;
});

test.afterAll(async () => {
  if (daemon?.exitCode === null && daemon?.signalCode === null) {
    const exited = once(daemon, 'exit');
    daemon.kill('SIGTERM');
    await exited;
  }
  if (root) rmSync(root, { recursive: true, force: true });
});

test('a pin on an HTML preview saves a boxed screenshot', async ({ page }) => {
  await expect.poll(async () => (await page.request.get(url + '/api/session')).status(), { timeout: 15_000 }).toBe(200);
  await page.goto(url + '/live');
  const frame = page.frameLocator('#hunkyardLiveIframe');
  await expect(frame.locator('#cta')).toBeVisible({ timeout: 15_000 });
  await expect.poll(() => page.evaluate(() => {
    const log = (window as unknown as { __hunkyardLiveMessages?: { type: string }[] }).__hunkyardLiveMessages;
    return Array.isArray(log) && log.some((e) => e.type === 'agent-ready');
  }), { timeout: 15_000 }).toBe(true);

  await enterPinMode(page);
  await frame.locator('#cta').click();
  await page.locator('.hunkyard-live-composer-body').fill('Trial button copy');
  await page.locator('.hunkyard-live-composer-save').click();
  await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);

  type Pin = { body: string; dom_anchor?: { pathname: string }; pin_screenshot?: { file: string; marker_rect: { x: number; y: number; height: number } } };
  let pin: Pin | undefined;
  await expect.poll(async () => {
    const all = await (await page.request.get(url + '/api/files/comments')).json() as Record<string, Pin[]>;
    pin = Object.values(all).flat().find((c) => c.body === 'Trial button copy');
    return pin?.pin_screenshot?.file;
  }, { timeout: 15_000 }).toBeTruthy();

  const shot = pin!.pin_screenshot!;
  const r = shot.marker_rect;
  const line = await page.evaluate(async ({ file, x, y }) => {
    const bitmap = await createImageBitmap(await (await fetch(`/api/attachments/${file}`)).blob());
    const canvas = document.createElement('canvas');
    canvas.width = bitmap.width;
    canvas.height = bitmap.height;
    const ctx = canvas.getContext('2d')!;
    ctx.drawImage(bitmap, 0, 0);
    return Array.from(ctx.getImageData(x, y, 1, 1).data.slice(0, 3));
  }, { file: shot.file, x: r.x - 8, y: Math.round(r.y + r.height / 2) });
  expect(line[0] > 220 && line[1] < 40 && line[2] < 40, `outline pixel ${line}`).toBe(true);
});
