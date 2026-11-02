import { test, expect } from '@playwright/test';
import { askFixtureDecision, clearAllComments, loadPage, submitReview } from './helpers';

test.describe('Decision requests from the agent', () => {
  test.beforeEach(async ({ request }) => {
    await clearAllComments(request);
  });

  test('questions are answered one page at a time, reviewed, then submitted together', async ({ page, request }) => {
    const auth = await askFixtureDecision(request, { question: 'Which auth strategy?', options: ['Session cookie', 'JWT'], context: { path: 'server.go', line: 3 } });
    const name = await askFixtureDecision(request, { question: 'What should the flag be called?', options: ['auth_v2', 'new_auth'] });
    await loadPage(page);
    const dock = page.locator('#decisionDock');
    await expect(dock.getByRole('region', { name: 'Questions from the agent' })).toBeVisible();
    await expect(dock.locator('.decision-progress')).toContainText('Question 1 of 2');
    await expect(dock.locator('.decision-question')).toHaveText('Which auth strategy?');

    await dock.getByRole('radio', { name: /JWT/ }).click();
    await expect(dock.locator('.decision-progress')).toContainText('Question 2 of 2');
    await dock.getByRole('textbox', { name: 'Your own answer' }).fill('auth_rollout');
    await dock.getByRole('button', { name: 'Review answers' }).click();

    const rows = dock.locator('.decision-review-row');
    await expect(rows).toHaveCount(2);
    await expect(rows.nth(0)).toContainText('JWT');
    await expect(rows.nth(1)).toContainText('auth_rollout');
    // Nothing reaches the agent before Submit.
    const before = await (await request.get('/api/decisions')).json();
    expect(before.every((d: { status: string }) => d.status === 'open')).toBe(true);

    await rows.nth(0).getByRole('button', { name: 'Change' }).click();
    await expect(dock.getByRole('radio', { name: /JWT/ })).toHaveAttribute('aria-checked', 'true');
    await page.keyboard.press('1'); // Session cookie
    await dock.getByRole('button', { name: 'Review answers' }).click();
    await expect(rows.nth(0)).toContainText('Session cookie');
    await dock.getByRole('button', { name: 'Submit answers' }).click();
    await expect(dock.locator('.decision-count')).toHaveText('All answered');
    await expect(dock.getByRole('status')).toHaveText('Sent to the agent');
    await expect(dock.locator('.decision-review-answer')).toHaveText(['auth_rollout', 'Session cookie']);
    await expect(dock.locator('.decision-earlier')).toHaveCount(0);

    // With nothing left to answer, the dock minimises once the pointer leaves it.
    await page.mouse.move(10, 10);
    await expect(dock.getByRole('button', { name: /Agent asks/ })).toBeVisible({ timeout: 10_000 });

    const finish = page.waitForResponse(r => r.url().endsWith('/api/finish'));
    await submitReview(page, 'comment');
    const { batch } = await (await finish).json();
    const byID = Object.fromEntries(batch.decisions.map((d: { id: string }) => [d.id, d]));
    expect(byID[auth.id]).toEqual(expect.objectContaining({ status: 'answered', chosen: 'Session cookie' }));
    expect(byID[name.id]).toEqual(expect.objectContaining({ status: 'answered', text: 'auth_rollout' }));
  });

  test('a blocking question holds back Approve until its answer is submitted', async ({ page, request }) => {
    await askFixtureDecision(request, { question: 'Ship behind a flag?', options: ['yes', 'no'], blocking: true });
    await loadPage(page);
    const dock = page.locator('#decisionDock');
    await expect(dock.locator('.decision-blocking')).toHaveText('Blocks approval');
    await page.locator('#finishBtn').click();
    const dialog = page.getByRole('dialog', { name: 'Submit review', exact: true });
    await expect(dialog.locator('input[name="disposition"][value="approve"]')).toBeDisabled();
    await expect(page.locator('#publishApproveBlocked')).toHaveText('1 blocking decision is unanswered.');
    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await dock.getByRole('radio', { name: /yes/ }).click();
    await dock.getByRole('button', { name: 'Submit answers' }).click();
    await expect(dock.locator('.decision-count')).toHaveText('All answered');
    await page.locator('#finishBtn').click();
    await expect(dialog.locator('input[name="disposition"][value="approve"]')).toBeEnabled();
  });

  test('the dock minimises to a launcher and dismissing is an answer', async ({ page, request }) => {
    await askFixtureDecision(request, { question: 'Rename the package?', options: ['yes', 'no'] });
    await loadPage(page);
    const dock = page.locator('#decisionDock');
    await dock.getByRole('button', { name: 'Minimise questions' }).click();
    const launcher = dock.getByRole('button', { name: /Agent asks/ });
    await expect(launcher).toHaveAttribute('aria-expanded', 'false');
    await launcher.click();
    await dock.getByRole('button', { name: 'Dismiss question' }).click();
    await expect(dock.locator('.decision-review-row')).toContainText('Dismissed');
    await dock.getByRole('button', { name: 'Submit answers' }).click();
    await expect(dock.locator('.decision-review-answer')).toHaveText('Dismissed');

    // A later question starts a new page; the dismissal moves to the folded history.
    await askFixtureDecision(request, { question: 'Split the PR?', options: ['yes', 'no'] });
    await expect(dock.locator('.decision-question')).toHaveText('Split the PR?');
    await dock.getByRole('radio', { name: /no/ }).click();
    await dock.getByRole('button', { name: 'Submit answers' }).click();
    await expect(dock.locator('.decision-review-answer').first()).toHaveText('no');
    const earlier = dock.locator('.decision-earlier');
    await expect(earlier).not.toHaveAttribute('open', '');
    await earlier.locator('summary').click();
    await expect(earlier).toContainText('Rename the package?');
  });
});
