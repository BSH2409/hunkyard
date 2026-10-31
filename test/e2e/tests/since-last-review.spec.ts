import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { clearAllComments, loadPage, goSection, fileHeader, publishFixtureReview, sessionCwd, signalRoundComplete } from './helpers';

const MARKER = '// since-last-review marker';

async function appendMarker(request: Parameters<typeof sessionCwd>[0]) {
  const file = path.join(await sessionCwd(request), 'server.go');
  const original = fs.readFileSync(file, 'utf8');
  const hashBefore = (await (await request.get('/api/file?path=server.go')).json()).file_hash;
  fs.writeFileSync(file, original + MARKER + '\n');
  // The agent's edit lands as a new round, as it would after review feedback.
  await expect(await signalRoundComplete(request)).toBeOK();
  // Wait for the new content and its hash, which the viewed state compares.
  await expect.poll(async () => {
    const file = await (await request.get('/api/file?path=server.go')).json();
    return file.content.includes(MARKER) && file.file_hash !== hashBefore;
  }, { timeout: 15_000 }).toBe(true);
  return async () => {
    fs.writeFileSync(file, original);
    await signalRoundComplete(request);
    // Wait for the server to reload the original, or the next test can mark
    // the file viewed at this marker's hash and append the same marker again.
    await expect.poll(async () => (await (await request.get('/api/file?path=server.go')).json()).content, { timeout: 15_000 }).not.toContain(MARKER);
  };
}

test('since last review shows only what changed after publishing', async ({ page, request }) => {
  await clearAllComments(request);
  await expect(await publishFixtureReview(request)).toBeOK();
  const restore = await appendMarker(request);
  try {
    await loadPage(page);
    const since = page.waitForResponse(r => r.url().includes('/api/file/diff?path=server.go') && r.url().includes('since=published'));
    await page.locator('#sinceToggle .toggle-btn[data-since="published"]').click();
    const body = await (await since).json();
    expect(body.since).toBe('published');
    const added = body.hunks.flatMap((h: { Lines: { Type: string; Content: string }[] }) => h.Lines).filter((l: { Type: string }) => l.Type === 'add');
    expect(added.map((l: { Content: string }) => l.Content)).toEqual([MARKER]);
    await expect((await goSection(page)).getByText(MARKER)).toBeVisible();
    await page.locator('#sinceToggle .toggle-btn[data-since="all"]').click();
    await expect(page.locator('#sinceToggle .toggle-btn[data-since="all"]')).toHaveAttribute('aria-pressed', 'true');
  } finally {
    await restore();
  }
});

test('a viewed file that changes afterwards is un-viewed and badged', async ({ page, request }) => {
  await clearAllComments(request);
  await loadPage(page);
  await goSection(page);
  const viewed = fileHeader(page, 'server.go').locator('.file-header-viewed input');
  await viewed.check();
  await expect(viewed).toBeChecked();
  // Leave the page so its own round reload cannot race the fresh load below.
  await page.goto('about:blank');
  const restore = await appendMarker(request);
  try {
    await loadPage(page);
    await goSection(page);
    await expect(fileHeader(page, 'server.go').locator('.file-header-changed')).toHaveText('Changed since you viewed');
    await expect(fileHeader(page, 'server.go').locator('.file-header-viewed input')).not.toBeChecked();
  } finally {
    await restore();
  }
});
