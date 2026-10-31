import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage } from './helpers';

test.beforeEach(async ({ request }) => { await clearAllComments(request); });

test('Yard tab lists a rule, shows health, and retires a rule', async ({ page }) => {
  await loadPage(page);
  const directive = `Write method definitions over multiple lines ${Date.now()}.`;
  const created = await page.request.post('/api/yard/rules', {
    headers: { 'Sec-Fetch-Site': 'same-origin' },
    data: { directive, scope: 'global', surfaces: ['ruby'], feedback: 'we dont use single line definitions' },
  });
  expect(created.ok()).toBeTruthy();

  await page.click('#settingsToggle');
  await page.click('.settings-tab[data-tab="yard"]');
  const yard = page.locator('.settings-pane[data-pane="yard"]');
  await expect(yard.locator('.yard-nav-item.active')).toHaveText('This review');
  await expect(yard.locator('.yard-event-kind', { hasText: 'learned' }).first()).toBeVisible();

  await yard.locator('[data-yard-view="rules"]').click();
  const row = yard.locator('.yard-row', { hasText: directive });
  await expect(row).toBeVisible();
  await expect(row.locator('.yard-label--global')).toBeVisible();

  await yard.locator('[data-yard-view="health"]').click();
  await expect(yard.locator('.yard-row-title', { hasText: 'store' })).toBeVisible();

  await yard.locator('[data-yard-view="rules"]').click();
  await yard.locator('.yard-row', { hasText: directive }).locator('[data-yard-action="retire"]').click();
  await expect(yard.locator('.yard-row', { hasText: directive })).toHaveCount(0);
});

test('Yard rejects changes that do not come from the page', async ({ request }) => {
  const response = await request.post('/api/yard/maintenance');
  expect(response.status()).toBe(403);
});
