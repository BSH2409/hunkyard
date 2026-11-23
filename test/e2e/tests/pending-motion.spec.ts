import { test, expect } from '@playwright/test';
import { clearAllComments, loadPage } from './helpers';

test.beforeEach(async ({ request }) => {
  await clearAllComments(request);
});

test('commit selection instructions remain readable', async ({ page }) => {
  await loadPage(page);
  await page.locator('#commitDropdownBtn').click();
  await expect(page.locator('.commit-picker-hint')).toHaveCSS('font-size', '12px');
});

test('pending comment respects reduced motion and keeps its status readable', async ({ page }) => {
  await loadPage(page);
  // The local-only binary cannot start an agent, so render its pending card directly.
  await page.evaluate(() => {
    const app = window as unknown as {
      markdownit: () => { render: (body: string) => string };
      hunkyard: {
        commentCard: {
          buildCommentCard: (comment: object, path: string, options: object) => { wrapper: HTMLElement };
        };
      };
    };
    const pending = app.hunkyard.commentCard.buildCommentCard(
      { id: 'motion-fixture', body: 'Pending feedback', author: 'Reviewer', review_round: 1 },
      '',
      { deps: { commentMd: app.markdownit() }, isPendingAgentRequest: () => true },
    );
    pending.wrapper.id = 'motion-fixture';
    document.querySelector('.main-content')!.prepend(pending.wrapper);
  });

  const pending = page.locator('#motion-fixture');
  const card = pending.locator('.comment-card');
  const badge = pending.locator('.live-thread-badge');
  const cursor = pending.locator('.agent-pending-cursor');
  await expect(pending).toHaveClass(/agent-pending/);
  await expect(card).toHaveCSS('animation-name', 'agent-glow');
  await expect(badge).toHaveCSS('animation-name', 'badge-pulse');
  await expect(cursor).toHaveCSS('animation-name', 'agent-cursor-blink');

  await page.emulateMedia({ reducedMotion: 'reduce' });
  await expect(card).toHaveCSS('animation-name', 'none');
  await expect(badge).toHaveCSS('animation-name', 'none');
  await expect(cursor).toHaveCSS('animation-name', 'none');
  await expect(pending.locator('.comment-round-badge')).toHaveCSS('font-size', '12px');
  await expect(badge).toHaveCSS('font-size', '12px');
  await expect(badge).toBeVisible();
  await expect(pending.locator('.agent-pending-author')).toBeVisible();
});
