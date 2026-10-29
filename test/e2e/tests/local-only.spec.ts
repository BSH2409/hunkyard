import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage, goSection, openLineComment } from './helpers';

test('review renders locally without attempting external requests', async ({ page, request }) => {
  const external: string[] = [];
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.context().route('**/*', route => {
    const url = new URL(route.request().url());
    if (['http:', 'https:'].includes(url.protocol) && !['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)) {
      external.push(url.origin);
      return route.abort();
    }
    return route.continue();
  });
  await loadPage(page);
  await expect((await goSection(page)).locator('.pierre-file-header')).toBeVisible();
  const response = await request.get('/');
  expect(response.headers()['content-security-policy']).toContain("connect-src 'self'");
  expect((await request.get('/api/config').then(response => response.json())).local_only).toBe(true);
  expect(external).toEqual([]);
  expect(errors).toEqual([]);
});

test('inline comments retain content identity and stale code is rejected', async ({ page, request }) => {
  await clearAllComments(request);
  await loadPage(page);
  const section = await goSection(page);
  const form = await openLineComment(page, section, 5);
  await form.locator('textarea').fill('Keep this boundary explicit.');
  const saved = page.waitForResponse(response => response.url().includes('/api/file/comments?') && response.request().method() === 'POST');
  await form.locator('.btn-primary').click();
  const response = await saved;
  expect(response.status()).toBe(201);
  const comment = await response.json();
  expect(comment.content_id).toMatch(/^sha256:/);
  expect(comment.view_id).toMatch(/^view_/);
  await expect(section.locator('.comment-card', { hasText: comment.body })).toBeVisible();
  const stale = await request.post('/api/file/comments?path=server.go', {
    data: { start_line: 5, end_line: 5, body: 'Stale draft', content_id: 'stale' },
  });
  expect(stale.status()).toBe(409);
});
