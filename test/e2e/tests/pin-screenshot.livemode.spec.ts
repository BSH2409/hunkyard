import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import { clearAllLivePins, enterPinMode, getIframe, setIframeRoute, waitAgentReadyAfterRoute, waitForAgentReady } from './livemode-helpers';
import { publishFixtureReview, signalRoundComplete } from './helpers';

// Pins capture the page inside the frame (SnapDOM) with a red box around the
// element, and the agent fetches the PNG from the review port.

const PROXY = `http://localhost:${Number(process.env.HUNKYARD_TEST_LIVE_PORT || '3129') + 1}`;

type Shot = { file: string; width: number; height: number; marker_rect: { x: number; y: number; width: number; height: number } };
type Pin = { id: string; body: string; pin_status?: string; pin_screenshot?: Shot; screenshot_status?: string; screenshot_reason?: string };

async function setPayLabel(request: APIRequestContext, label: string) {
  await expect(await request.post(`${PROXY}/modern/toggle?label=${encodeURIComponent(label)}`)).toBeOK();
}

async function pinOn(page: Page, route: string, selector: string, body: string) {
  await waitForAgentReady(page);
  await setIframeRoute(page, route);
  await waitAgentReadyAfterRoute(page);
  await enterPinMode(page);
  const target = getIframe(page).locator(selector);
  await target.scrollIntoViewIfNeeded();
  await target.click();
  await expect(page.locator('.hunkyard-live-composer')).toBeVisible();
  await page.locator('.hunkyard-live-composer-body').fill(body);
  await page.locator('.hunkyard-live-composer-save').click();
  await expect(page.locator('.hunkyard-live-composer')).toHaveCount(0);
}

async function savedPin(request: APIRequestContext, route: string, body: string): Promise<Pin> {
  let pin: Pin | undefined;
  await expect.poll(async () => {
    const res = await request.get(`/api/file/comments?path=${encodeURIComponent(route)}`);
    if (!res.ok()) return undefined;
    pin = (await res.json() as Pin[]).find((c) => c.body === body);
    return pin?.pin_screenshot?.file ?? pin?.screenshot_status;
  }, { timeout: 15_000 }).toBeTruthy();
  return pin!;
}

// Reads pixels of an attachment in the review page, which may fetch it
// (same origin), as [r, g, b] per point.
async function pixels(page: Page, file: string, points: Array<[number, number]>): Promise<number[][]> {
  return page.evaluate(async ({ file, points }) => {
    const res = await fetch(`/api/attachments/${file}`);
    if (!res.ok) throw new Error(`attachment ${res.status}`);
    const bitmap = await createImageBitmap(await res.blob());
    const canvas = document.createElement('canvas');
    canvas.width = bitmap.width;
    canvas.height = bitmap.height;
    const ctx = canvas.getContext('2d')!;
    ctx.drawImage(bitmap, 0, 0);
    return points.map(([x, y]) => Array.from(ctx.getImageData(x, y, 1, 1).data.slice(0, 3)));
  }, { file, points });
}

const isRed = (p: number[]) => p[0] > 220 && p[1] < 40 && p[2] < 40;
const isWhite = (p: number[]) => p[0] > 220 && p[1] > 220 && p[2] > 220;

test.describe('pin screenshots', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllLivePins(request);
    await setPayLabel(request, 'Pay now');
  });

  test('a pin on a modern-CSS page stores a PNG with the element boxed in red', async ({ page, request }) => {
    await pinOn(page, '/modern', '#pay', 'Pay button too small');
    const pin = await savedPin(request, '/modern', 'Pay button too small');
    expect(pin.screenshot_status ?? '').toBe('');
    const shot = pin.pin_screenshot!;
    // The whole 2,300 px page, not just the viewport.
    expect(shot.height).toBeGreaterThan(2000);
    const r = shot.marker_rect;
    expect(r.y).toBeGreaterThan(1800);
    const midY = Math.round(r.y + r.height / 2);
    // The outline is centred 8 px (padding 6 + stroke/2) outside the element;
    // its white halo runs 2 px beyond the 4 px red line on both sides.
    const [line, outsideHalo, insideHalo, element] = await pixels(page, shot.file, [
      [r.x - 8, midY], [r.x - 11, midY], [r.x - 5, midY], [r.x + 4, midY],
    ]);
    expect(isRed(line), `outline pixel ${line}`).toBe(true);
    expect(isWhite(outsideHalo), `outer halo pixel ${outsideHalo}`).toBe(true);
    expect(isWhite(insideHalo), `inner halo pixel ${insideHalo}`).toBe(true);
    expect(isRed(element), `element pixel ${element}`).toBe(false);
  });

  test('a pin inside an app that scrolls in a container is boxed inside the image', async ({ page, request }) => {
    await pinOn(page, '/scroll-app', '#deep', 'Deep action label');
    const pin = await savedPin(request, '/scroll-app', 'Deep action label');
    expect(pin.screenshot_reason ?? '', 'capture failure reason').toBe('');
    const shot = pin.pin_screenshot!;
    const r = shot.marker_rect;
    expect(r.x).toBeGreaterThanOrEqual(0);
    expect(r.y).toBeGreaterThan(1500);
    expect(r.x + r.width).toBeLessThanOrEqual(shot.width);
    expect(r.y + r.height).toBeLessThanOrEqual(shot.height);
    const [line] = await pixels(page, shot.file, [[r.x - 8, Math.round(r.y + r.height / 2)]]);
    expect(isRed(line), `outline pixel ${line}`).toBe(true);
  });

  test('a pin whose element changed reads Outdated after the next round', async ({ page, request }) => {
    await pinOn(page, '/modern', '#pay', 'Make it say Pay now');
    const pin = await savedPin(request, '/modern', 'Make it say Pay now');
    // A new pin is checked on the page it was made on, without a new round.
    await expect.poll(async () => {
      const res = await request.get('/api/file/comments?path=%2Fmodern');
      return (await res.json() as Pin[]).find((c) => c.id === pin.id)?.pin_status;
    }, { timeout: 15_000 }).toBe('current');
    const row = page.locator(`#commentsPanelBody [data-comment-id="${pin.id}"]`);
    await expect(row.locator('.outdated-badge')).toHaveCount(0);

    await setPayLabel(request, 'Pay later');
    await expect(await publishFixtureReview(request)).toBeOK();
    await expect(await signalRoundComplete(request)).toBeOK();
    await setIframeRoute(page, '/modern');
    await waitAgentReadyAfterRoute(page);

    await expect.poll(async () => {
      const res = await request.get('/api/file/comments?path=%2Fmodern');
      return (await res.json() as Pin[]).find((c) => c.id === pin.id)?.pin_status;
    }, { timeout: 15_000 }).toBe('outdated');
    await expect(row.locator('.outdated-badge')).toHaveText('Outdated');

    // The next batch carries the reviewer's follow-up on the pin, with the
    // pin's current state.
    await expect(await request.post(`/api/comment/${pin.id}/replies?path=%2Fmodern`, { data: { body: 'Still says Pay later' } })).toBeOK();
    const finish = await publishFixtureReview(request);
    await expect(finish).toBeOK();
    const { batch } = await finish.json() as { batch: { comments: Array<{ id: string; parent_id?: string; outdated?: boolean; pin?: { status: string } }> } };
    const published = batch.comments.find((c) => c.parent_id === pin.id);
    expect(published?.pin?.status).toBe('outdated');
    expect(published?.outdated).toBe(true);
  });

  test('another site cannot read a pin screenshot; the review page can', async ({ page, request }) => {
    await pinOn(page, '/modern', '#pay', 'Screenshot privacy');
    const { file } = (await savedPin(request, '/modern', 'Screenshot privacy')).pin_screenshot!;
    const apiOrigin = new URL(page.url()).origin;

    // An <img> needs no CORS, so only the server's Sec-Fetch-Site check can
    // stop the app from showing it.
    const loadImage = (url: string) => new Promise<string>((resolve) => {
      const img = new Image();
      img.onload = () => resolve('loaded');
      img.onerror = () => resolve('refused');
      img.src = url;
    });
    const url = `${apiOrigin}/api/attachments/${file}`;
    // Positive control: the review page (same origin) loads it.
    expect(await page.evaluate(`(${loadImage.toString()})(${JSON.stringify(url)})`)).toBe('loaded');
    // The reviewed app runs on the proxy port: same site, another origin.
    const fromApp = await getIframe(page).locator('body').evaluate(
      (_body, src) => new Function(`return (${src.fn})(${JSON.stringify(src.url)})`)(),
      { fn: loadImage.toString(), url },
    );
    expect(fromApp).toBe('refused');
    const direct = await request.get(`/api/attachments/${file}`, { headers: { 'Sec-Fetch-Site': 'same-site' } });
    expect(direct.status()).toBe(403);
  });
});
