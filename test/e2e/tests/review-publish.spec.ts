import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage, goSection, openLineComment } from './helpers';

test('review submission preserves an unfinished inline draft', async ({ page, request }) => {
  await clearAllComments(request);
  await loadPage(page);
  const section = await goSection(page);
  const form = await openLineComment(page, section, 5);
  const text = form.locator('textarea');
  await text.fill('Keep this draft until I save the comment.');
  await page.locator('#finishBtn').click();
  await expect(text).toBeFocused();
  await expect(page.getByRole('dialog', { name: 'Submit review', exact: true })).toBeHidden();
  await expect(text).toHaveValue('Keep this draft until I save the comment.');
  const saved = page.waitForResponse(response => response.url().includes('/api/file/comments?') && response.request().method() === 'POST');
  await form.locator('.btn-primary').click();
  expect((await saved).status()).toBe(201);
  await page.locator('#finishBtn').click();
  await expect(page.getByRole('dialog', { name: 'Submit review', exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
});

test('publishing requires an explicit decision and persists one batch per request', async ({ page, request }) => {
  await clearAllComments(request);
  await loadPage(page);
  await expect(page.locator('#finishBtn')).toHaveText('Review changes');
  await page.locator('#finishBtn').click();
  const dialog = page.getByRole('dialog', { name: 'Submit review', exact: true });
  await expect(dialog).toBeVisible();
  await expect(page.locator('#publishSummary')).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
  await page.locator('#finishBtn').click();
  await page.locator('#publishSummary').fill('Prefer clear function names in Go.');
  await dialog.getByLabel('Request changes', { exact: true }).check();
  const posted = page.waitForResponse(response => response.url().endsWith('/api/finish') && response.request().method() === 'POST');
  await dialog.getByRole('button', { name: 'Submit review', exact: true }).click();
  const response = await posted;
  expect(response.status()).toBe(200);
  expect(JSON.parse(response.request().postData()!).displayed_files).toEqual(expect.arrayContaining([expect.objectContaining({ path: expect.any(String), content_id: expect.any(String), view_id: expect.stringMatching(/^view_/) })]));
  const data = await response.json();
  expect(data.approved).toBe(false);
  expect(data.batch.disposition).toBe('request_changes');
  await expect(page.locator('#waitingHeading')).toHaveText('Review submitted');
  await expect(page.locator('#tipSection')).toHaveCount(0);
  expect(data.batch.comments).toEqual(expect.arrayContaining([expect.objectContaining({ id: 'review-summary', revision: 1 })]));
  const replay = await request.post('/api/finish', { data: JSON.parse(response.request().postData()!) });
  expect(replay.status()).toBe(200);
  expect((await replay.json()).batch.id).toBe(data.batch.id);
  const unauthorized = await request.get('/api/feedback/await?timeout=1ms');
  expect(unauthorized.status()).toBe(403);
});

test('changes made after publishing relabel the button until they are sent', async ({ page, request }) => {
  await clearAllComments(request);
  await loadPage(page);
  await page.locator('#finishBtn').click();
  const dialog = page.getByRole('dialog', { name: 'Submit review', exact: true });
  await dialog.getByLabel('Comment', { exact: true }).check();
  await dialog.getByRole('button', { name: 'Submit review', exact: true }).click();
  await expect(page.locator('#waitingHeading')).toHaveText('Review submitted');
  await page.locator('#backToEditing').click();
  await expect(page.locator('#finishBtn')).toHaveText('Review changes');

  // A comment added after publishing has not reached the agent yet.
  const form = await openLineComment(page, await goSection(page), 5);
  await form.locator('textarea').fill('One more thing.');
  await form.locator('.btn-primary').click();
  await expect(page.locator('#finishBtn')).toHaveText('Send update');
});

test('skill picker shows only host IDs as text and preserves a cancelled selection', async ({ page, request }) => {
  await clearAllComments(request);
  await page.route('**/api/skills', route => route.fulfill({ json: { skills: [{ id: 'naming', name: '<img src=x onerror=alert(1)>', description: 'Check names' }] } }));
  await loadPage(page);
  await page.locator('#finishBtn').click();
  const dialog = page.getByRole('dialog', { name: 'Submit review', exact: true });
  const skill = dialog.getByRole('checkbox', { name: '<img src=x onerror=alert(1)>' });
  await expect(skill).toBeVisible();
  await expect(dialog.locator('#publishSkills img')).toHaveCount(0);
  await skill.check();
  await page.keyboard.press('Escape');
  await page.locator('#finishBtn').click();
  await expect(skill).toBeChecked();
  await page.keyboard.press('Escape');
});

test.describe('Review interface cleanup', () => {
  test.beforeEach(async ({ request }) => { await clearAllComments(request); });

  test('dialog keeps its controls visible on a narrow screen with five skills', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.route('**/api/skills', route => route.fulfill({ json: { skills: [
      { id: 'code', name: 'Code Change' }, { id: 'review', name: 'Review Change' },
      { id: 'naming', name: 'Naming Analyzer' }, { id: 'preferences', name: 'Learn Preferences' },
      { id: 'ponytail', name: 'Ponytail' },
    ] } }));
    await loadPage(page);
    await page.locator('#finishBtn').click();
    const dialog = page.getByRole('dialog', { name: 'Submit review', exact: true });
    await expect(dialog.getByRole('group', { name: 'Skills to apply' })).toBeVisible();
    await expect(dialog.getByRole('checkbox')).toHaveCount(5);
    await expect(dialog.getByRole('button', { name: 'Submit review', exact: true })).toBeVisible();
    await expect.poll(() => dialog.evaluate(el => {
      const box = el.getBoundingClientRect();
      return box.left >= 0 && box.right <= innerWidth && box.top >= 0 && box.bottom <= innerHeight;
    })).toBe(true);
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(dialog).toBeHidden();
  });

  test('about links point to Hunkyard and the waiting UI has no tips', async ({ page }) => {
    await loadPage(page);
    await expect(page.locator('#tipSection, #tipText')).toHaveCount(0);
    await page.locator('#settingsToggle').click();
    await page.getByRole('tab', { name: 'About', exact: true }).click();
    await expect(page.locator('#aboutPane .about-link')).toHaveCount(2);
    await expect(page.locator('#aboutPane').getByRole('link', { name: 'Project source' })).toHaveAttribute('href', 'https://github.com/BSH2409/hunkyard');
    await expect(page.locator('#aboutPane').getByRole('link', { name: 'Documentation' })).toHaveAttribute('href', 'https://github.com/BSH2409/hunkyard#readme');
  });
});
