import { test, expect } from '@playwright/test';

test('standalone Yard works without a review session', async ({ page, request }) => {
  expect((await request.get('/api/session')).status()).toBe(503);
  await page.goto('/yard');
  const yard = page.locator('.yard--page');
  await expect(yard.locator('.yard-nav-item')).toHaveText(['Rules', 'Pipeline', 'Persona', 'Loads', 'Activity', 'Health']);
  await expect(yard.locator('.yard-row', { hasText: 'Write method definitions over multiple lines.' })).toBeVisible();
  await expect(yard.locator('.yard-row', { hasText: 'Keep test files out of commits.' }).locator('.yard-label--repo')).toHaveText('orion');

  await yard.locator('[data-yard-view="persona"]').click();
  await expect(yard.locator('[data-persona-file="core.md"]')).toBeVisible();
  await yard.locator('[data-persona-file="ruby.md"]').click();
  await expect(yard.locator('.yard-file-view')).toContainText('multiple lines');

  await yard.locator('[data-yard-view="activity"]').click();
  await expect(yard.locator('.yard-event-kind', { hasText: 'learned' }).first()).toBeVisible();
});
