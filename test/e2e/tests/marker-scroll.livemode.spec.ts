// Regression coverage for 2f6afe7 ("anchor pin marker to document not viewport").
//
// Pre-fix the marker was `position: fixed` and only recomputed on
// MutationObserver callbacks — scrolling the proxied page did NOT move the
// marker, so it drifted away from the element it annotated. The fix moved
// markers to `position: absolute` in document coords (under #hunkyard-marker-root
// at the document origin, transform = rect + scrollY/scrollX).
//
// Test contract: pin a target, scroll the iframe, and assert the target's
// viewport-Y and the marker's viewport-Y change by the SAME delta. If the
// marker were viewport-anchored again, its viewport-Y would stay constant
// while the target's dropped — the difference would diverge.
import { test, expect } from '@playwright/test';
import { clearAllLivePins, enterPinMode, getIframe, openPinComposer, setIframeRoute, waitAgentReadyAfterRoute, waitForAgentReady } from './livemode-helpers';

test.describe('live-mode markers — scroll anchoring', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllLivePins(request);
  });

  test('marker tracks its element when the iframe scrolls', async ({ page }) => {
    // Pin the primary button on the upstream root.
    await openPinComposer(page, '#primary-btn');
    await page.locator('.hunkyard-live-composer-body').fill('scroll anchor pin');
    await page.locator('.hunkyard-live-composer-save').click();
    await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);

    const ifr = getIframe(page);
    const target = ifr.locator('#primary-btn');
    const marker = ifr.locator('.hunkyard-live-marker').first();
    await expect(marker).toBeVisible();

    // Make the iframe document tall enough to scroll. Inject a tall spacer
    // BEFORE the target so scrolling the document moves both the target
    // and (if correctly anchored) the marker. Element identity is preserved.
    await ifr.locator('body').evaluate((body) => {
      const spacer = document.createElement('div');
      spacer.id = '__hunkyard_test_spacer';
      spacer.style.height = '1500px';
      spacer.style.background = 'transparent';
      body.insertBefore(spacer, body.firstChild);
    });

    // Wait for the agent's MutationObserver to re-anchor the marker to the
    // target's new document-relative position. Without this we'd race the
    // observer and the "before" measurement could capture the marker still
    // at its pre-spacer location.
    await expect.poll(
      async () => {
        const t = await target.boundingBox();
        const m = await marker.boundingBox();
        if (!t || !m) return null;
        // Marker anchors at the element's top-left corner; settle when both
        // are within a few px of each other vertically.
        return Math.abs(t.y - m.y) < 5;
      },
      { timeout: 5_000 },
    ).toBe(true);

    // Pre-scroll measurement.
    const before = {
      target: await target.boundingBox(),
      marker: await marker.boundingBox(),
    };
    expect(before.target).not.toBeNull();
    expect(before.marker).not.toBeNull();

    // Scroll the iframe document by a fixed amount.
    const scrollDelta = 600;
    await ifr.locator('body').evaluate((_, dy) => {
      window.scrollTo(0, dy);
    }, scrollDelta);

    // Both the target and the marker must shift by approximately the same
    // amount in viewport coords. Allow a small tolerance for sub-pixel
    // rounding inside transform translation.
    await expect.poll(
      async () => {
        const t = await target.boundingBox();
        const m = await marker.boundingBox();
        if (!t || !m || !before.target || !before.marker) return null;
        return {
          targetDelta: Math.round(before.target.y - t.y),
          markerDelta: Math.round(before.marker.y - m.y),
        };
      },
      { timeout: 5_000 },
    ).toEqual({ targetDelta: scrollDelta, markerDelta: scrollDelta });
  });
});

test.describe('live-mode markers — apps that scroll in a panel and re-render', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllLivePins(request);
  });

  test('a marker follows its element when a panel scrolls and when the app re-renders it', async ({ page }) => {
    await waitForAgentReady(page);
    await setIframeRoute(page, '/scroll-app');
    await waitAgentReadyAfterRoute(page);
    await enterPinMode(page);
    const ifr = getIframe(page);
    await ifr.locator('#deep').scrollIntoViewIfNeeded();
    await ifr.locator('#deep').click();
    await page.locator('.hunkyard-live-composer-body').fill('panel pin');
    await page.locator('.hunkyard-live-composer-save').click();
    await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);

    const marker = ifr.locator('.hunkyard-live-marker').first();
    const nearTarget = async () => {
      const t = await ifr.locator('#deep').boundingBox();
      const m = await marker.boundingBox();
      return !!t && !!m && Math.abs(t.y - m.y) < 5 && Math.abs(t.x - m.x) < 5;
    };
    await expect.poll(nearTarget, { timeout: 5_000 }).toBe(true);

    // Scrolling the panel changes no DOM; the marker must still follow.
    await ifr.locator('#app').evaluate((el) => {
      const deep = document.getElementById('deep')!;
      el.scrollTop = deep.offsetTop - 300;
    });
    await expect.poll(nearTarget, { timeout: 5_000 }).toBe(true);

    // Scrolled out of the panel's view, the marker is hidden, not drawn over
    // whatever the panel shows; scrolled back, it returns.
    await ifr.locator('#app').evaluate((el) => { el.scrollTop = 0; });
    await expect(marker).toBeHidden({ timeout: 5_000 });
    await ifr.locator('#app').evaluate((el) => {
      const deep = document.getElementById('deep')!;
      el.scrollTop = deep.offsetTop - 300;
    });
    await expect.poll(nearTarget, { timeout: 5_000 }).toBe(true);

    // The app replaces the element with an identical one (a re-render).
    await ifr.locator('#deep').evaluate((el) => { el.replaceWith(el.cloneNode(true)); });
    await expect(marker).toBeVisible({ timeout: 5_000 });
    await expect.poll(nearTarget, { timeout: 5_000 }).toBe(true);
  });
});

test.describe('live-mode markers — Comment mode', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllLivePins(request);
  });

  test('in Comment mode a marker is dimmed and a click on it picks the element underneath', async ({ page }) => {
    await openPinComposer(page, '#primary-btn');
    await page.locator('.hunkyard-live-composer-body').fill('under the marker');
    await page.locator('.hunkyard-live-composer-save').click();
    await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);
    const ifr = getIframe(page);
    const marker = ifr.locator('.hunkyard-live-marker').first();
    await expect(marker).toBeVisible();
    await enterPinMode(page);
    await expect(marker).toHaveCSS('pointer-events', 'none');
    await expect(marker).toHaveCSS('opacity', '0.45');
    const box = (await marker.boundingBox())!;
    await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
    await expect(page.locator('.hunkyard-live-composer')).toBeVisible();
    await page.locator('.hunkyard-live-composer-cancel').click();
  });
});

test.describe('live-mode markers — pages that build their content after load', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllLivePins(request);
  });

  test('after a reload a marker is hidden until its element exists, then sits on it', async ({ page }) => {
    await waitForAgentReady(page);
    await setIframeRoute(page, '/late');
    await waitAgentReadyAfterRoute(page);
    const ifr = getIframe(page);
    await expect(ifr.locator('#late-btn')).toBeVisible();
    await enterPinMode(page);
    await ifr.locator('#late-btn').click();
    await page.locator('.hunkyard-live-composer-body').fill('late pin');
    await page.locator('.hunkyard-live-composer-save').click();
    await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);

    // Hard reload: the button is gone for 700 ms while the page builds itself.
    await setIframeRoute(page, '/late?reload=1');
    await waitAgentReadyAfterRoute(page);
    const marker = ifr.locator('.hunkyard-live-marker').first();
    if (await ifr.locator('#late-btn').count() === 0) {
      // Not parked in the top-left corner while its element is missing.
      await expect(marker).toBeHidden();
    }
    await expect(ifr.locator('#late-btn')).toBeVisible();
    await expect.poll(async () => {
      const t = await ifr.locator('#late-btn').boundingBox();
      const m = await marker.boundingBox();
      return !!t && !!m && Math.abs(t.y - m.y) < 5 && Math.abs(t.x - m.x) < 5;
    }, { timeout: 5_000 }).toBe(true);
  });
});

test.describe('live-mode markers — pages whose body scrolls', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllLivePins(request);
  });

  test('a marker sits on its element on a page whose offset body scrolls instead of the window', async ({ page }) => {
    await waitForAgentReady(page);
    await setIframeRoute(page, '/body-scroll');
    await waitAgentReadyAfterRoute(page);
    const ifr = getIframe(page);
    await enterPinMode(page);
    await ifr.locator('#body-target').scrollIntoViewIfNeeded();
    await ifr.locator('#body-target').click();
    await page.locator('.hunkyard-live-composer-body').fill('body scroll pin');
    await page.locator('.hunkyard-live-composer-save').click();
    await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);
    const marker = ifr.locator('.hunkyard-live-marker').first();
    const onTarget = async () => {
      const t = await ifr.locator('#body-target').boundingBox();
      const m = await marker.boundingBox();
      return !!t && !!m && Math.abs(t.y - m.y) < 5 && Math.abs(t.x - m.x) < 5;
    };
    await expect.poll(onTarget, { timeout: 5_000 }).toBe(true);
    await ifr.locator('body').evaluate((b) => { b.scrollTop -= 150; });
    await expect.poll(onTarget, { timeout: 5_000 }).toBe(true);
  });
});

test.describe('live-mode markers — panels that cover the element', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllLivePins(request);
  });

  test('a marker hides while a panel covers its element and comes back when it closes', async ({ page }) => {
    await waitForAgentReady(page);
    await setIframeRoute(page, '/covered');
    await waitAgentReadyAfterRoute(page);
    const ifr = getIframe(page);
    await enterPinMode(page);
    await ifr.locator('#covered-target').click();
    await page.locator('.hunkyard-live-composer-body').fill('covered pin');
    await page.locator('.hunkyard-live-composer-save').click();
    await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);
    const marker = ifr.locator('.hunkyard-live-marker').first();
    await expect(marker).toBeVisible();

    await page.locator('#liveModeToggle button[data-mode="navigate"]').click();
    await ifr.locator('#toggle-panel').click();
    await expect(ifr.locator('#drawer')).toBeVisible();
    await expect(marker).toBeHidden();
    await ifr.locator('#toggle-panel').click();
    await expect(ifr.locator('#drawer')).toBeHidden();
    await expect(marker).toBeVisible();
  });
});
