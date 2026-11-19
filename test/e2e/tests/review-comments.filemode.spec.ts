import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage } from './helpers';

test.describe('Review-level comments — File Mode', () => {
  test.beforeEach(async ({ request, page }) => {
    await clearAllComments(request);
    await loadPage(page);
  });

  test('G shortcut opens, creates, and deletes a review comment in file mode', async ({ page, request }) => {
    await page.keyboard.press('Shift+G');
    const textarea = page.locator('#reviewConversation .review-chat-composer textarea');
    await expect(textarea).toBeVisible();
    await expect(textarea).toBeFocused();

    await textarea.fill('File mode review comment');
    await page.locator('#reviewConversation .review-chat-composer .btn-primary').click();

    const cards = page.locator('#reviewConversation .comment-card');
    await expect(cards).toHaveCount(1);
    await expect(cards.first()).toContainText('File mode review comment');

    await cards.first().locator('.delete-btn').click();
    await expect(page.locator('#reviewConversation .comment-card')).toHaveCount(0);
  });

  test('review conversation floats over the document and preserves a draft', async ({ page }) => {
    await page.locator('#reviewChatToggle').click();
    const panel = page.locator('#reviewChatPanel');
    await expect(panel).toBeVisible();
    const textarea = panel.locator('textarea');
    await textarea.fill('Keep my review draft');
    await panel.getByRole('button', { name: 'Close', exact: true }).click();
    await expect(panel).toBeHidden();
    await page.locator('#reviewChatToggle').click();
    await expect(panel.locator('textarea')).toHaveValue('Keep my review draft');
    const box = await panel.boundingBox();
    expect(box!.width).toBeGreaterThanOrEqual(600);
    expect(box!.height).toBeGreaterThanOrEqual(500);
  });
});
