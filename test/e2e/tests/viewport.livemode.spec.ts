import { test, expect } from '@playwright/test';
import {
  clearAllLivePins,
  getIframe,
  openPinComposer,
  setViewportPreset,
} from './livemode-helpers';

test.describe('viewport — preset round-trip (Scenario 9)', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllLivePins(request);
  });

  test('marker re-resolves after viewport change', async ({ page }) => {
    await openPinComposer(page);
    await page.locator('.hunkyard-live-composer-body').fill('Pin');
    await page.locator('.hunkyard-live-composer-save').click();
    await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);
    await expect(getIframe(page).locator('.hunkyard-live-marker')).toHaveCount(1);
    await setViewportPreset(page, 'mobile');
    // After viewport change the marker still tracks one element.
    await expect(getIframe(page).locator('.hunkyard-live-marker')).toHaveCount(1);
    await expect(getIframe(page).locator('.hunkyard-live-marker')).toBeVisible();
  });
});
