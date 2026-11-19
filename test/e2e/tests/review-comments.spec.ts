import { test, expect, type APIRequestContext } from '@playwright/test';
import { clearAllComments, loadPage } from './helpers';

async function seedReviewComment(request: APIRequestContext, body: string) {
  await expect(await request.post('/api/comments', { data: { body } })).toBeOK();
}

test.describe('Review-level comments — Git Mode', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllComments(request);
  });

  test.describe('starting empty', () => {
    test.beforeEach(async ({ page }) => {
      await loadPage(page);
    });

    test('G focuses a persistent composer; Escape preserves it and sending clears it', async ({ page }) => {
      await page.keyboard.press('Shift+G');
      const textarea = page.locator('#reviewConversation .review-chat-composer textarea');
      await expect(textarea).toBeVisible();
      await expect(textarea).toBeFocused();
      await textarea.fill('keep this draft');
      await textarea.press('Escape');
      await expect(page.locator('#reviewChatPanel')).toBeHidden();
      await page.keyboard.press('Shift+G');
      await expect(textarea).toHaveValue('keep this draft');
      await textarea.fill('submitted with ctrl+enter');
      await textarea.press('Control+Enter');
      await expect(page.locator('#reviewConversation .comment-card')).toHaveCount(1);
      await expect(textarea).toHaveValue('');
      await expect(textarea).toBeVisible();
    });

    test('conversation navigation opens the empty composer and Submit adds a comment', async ({ page }) => {
      await expect(page.locator('#reviewConversation')).toBeHidden();
      await page.locator('.tree-conversation-row').click();

      const textarea = page.locator('#reviewConversation .review-chat-composer textarea');
      await expect(textarea).toBeVisible();
      await expect(textarea).toBeFocused();
      await textarea.fill('General feedback');
      await page.locator('#reviewConversation .review-chat-composer .btn-primary').click();

      const cards = page.locator('#reviewConversation .comment-card');
      await expect(cards).toHaveCount(1);
      await expect(cards.first()).toContainText('General feedback');
    });
  });

  test.describe('with a seeded review comment', () => {
    test('renders on load, counts in the header, and shows in the tree conversation row', async ({ page, request }) => {
      await seedReviewComment(request, 'api review comment');
      await loadPage(page);

      await expect(page.locator('#reviewConversation .comment-card')).toHaveCount(1);
      await expect(page.locator('#reviewConversation .comment-card').first()).toContainText('api review comment');
      await expect(page.locator('#commentCountNumber')).toHaveText('1');

      const row = page.locator('.tree-conversation-row');
      await expect(row).toBeVisible();
      await expect(row).toContainText('Review conversation');
      await expect(row.locator('.tree-conversation-badge')).toHaveText('1');
    });

    test('can edit, reply to, and delete a review comment', async ({ page, request }) => {
      await seedReviewComment(request, 'original review');
      await loadPage(page);
      await page.locator('#reviewChatToggle').click();

      const card = page.locator('#reviewConversation .comment-card').first();
      await expect(card).toBeVisible();

      await card.locator('button[title="Edit"]').click();
      const textarea = page.locator('#reviewConversation .comment-form textarea').first();
      await expect(textarea).toHaveValue('original review');
      await textarea.fill('updated review');
      await page.locator('#reviewConversation .comment-form .btn-primary').first().click();
      await expect(page.locator('#reviewConversation .comment-card .comment-body')).toContainText('updated review');

      await card.getByRole('button', { name: 'Reply', exact: true }).click();
      const replyTextarea = page.locator('#reviewConversation .review-chat-composer textarea');
      await replyTextarea.fill('a reply');
      await page.getByRole('button', { name: 'Send message', exact: true }).click();
      await expect(page.locator('#reviewConversation .reply-body')).toContainText('a reply');

      await card.locator('.delete-btn').first().click();
      await expect(page.locator('#reviewConversation .comment-card')).toHaveCount(0);
    });

    test('clicking a panel review-comment card navigates to and flashes the conversation card', async ({ page, request }) => {
      await seedReviewComment(request, 'panel navigation target');
      await loadPage(page);

      await page.keyboard.press('Shift+C');
      await expect(page.locator('#commentsPanel')).not.toHaveClass(/comments-panel-hidden/);

      const panelCard = page.locator('#commentsPanelBody .comment-card').first();
      await expect(panelCard).toBeVisible();
      await panelCard.click();

      // Keep navigation feedback visible inside the floating conversation.
      await expect(page.locator('.tree-conversation-row')).toHaveClass(/active/);
      await expect(page.locator('#reviewConversation .comment-card.comment-card-highlight')).toBeVisible();
    });

    test('conversation closes and reopens without losing threads', async ({ page, request }) => {
      await seedReviewComment(request, 'a thread');
      await loadPage(page);
      await page.locator('#reviewChatToggle').click();
      await expect(page.locator('#reviewConversation .comment-card')).toBeVisible();
      await page.locator('.review-chat-close').click();
      await expect(page.locator('#reviewChatPanel')).toBeHidden();
      await expect(page.locator('#reviewChatToggle')).toBeFocused();
      await page.locator('#reviewChatToggle').click();
      await expect(page.locator('#reviewConversation .comment-card')).toBeVisible();
    });
  });
});
