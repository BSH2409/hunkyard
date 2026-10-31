import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { clearAllComments, loadPage, sessionCwd } from './helpers';

const MARKER = 'Hand edit outside the round';

test.beforeEach(async ({ request }) => {
  await clearAllComments(request);
});

test('a hand edit shows a Refresh banner and loads on Refresh', async ({ page, request }) => {
  const file = path.join(await sessionCwd(request), 'plan.md');
  const original = fs.readFileSync(file, 'utf8');
  await loadPage(page);
  const banner = page.locator('#reviewUpdatesBanner');
  await expect(banner).toBeHidden();
  try {
    fs.writeFileSync(file, original + '\n' + MARKER + '\n');
    await expect(banner).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText(MARKER)).toHaveCount(0);
    await page.locator('#reviewUpdatesRefresh').click();
    await expect(banner).toBeHidden();
    await expect(page.getByText(MARKER).first()).toBeVisible();
  } finally {
    fs.writeFileSync(file, original);
    await expect.poll(async () => (await (await request.get('/api/file?path=plan.md')).json()).content).toBe(original);
    await request.post('/api/refresh');
  }
});
