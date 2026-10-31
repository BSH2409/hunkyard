import { test, expect, type Page } from '@playwright/test';
import { loadPage, clearAllComments, selectedLines } from './helpers';

// The overlay shell (open/close, Esc, backdrop click, ?, tab switching and its
// scroll reset, focus trap) is unit-tested in web/__tests__/hunkyard-settings-overlay.test.js,
// and pane markup in hunkyard-settings-panes.test.js. These tests cover the wiring
// in the real page: which tab opens, what each pane shows, and settings that
// change the page and persist across reloads.

// Keyboard focus: a Pierre-selected diff line, or a focused document block.
function keyboardFocus(page: Page) {
  return page.locator('.kb-nav.focused').or(selectedLines(page));
}

const overlay = (page: Page) => page.locator('.settings-overlay');

test.describe('Settings Panel', () => {
  test.beforeEach(async ({ page, request }) => {
    await clearAllComments(request);
    await loadPage(page);
  });

  test('the dialog keeps one height across tabs and tab labels are centred', async ({ page }) => {
    await page.click('#settingsToggle');
    await expect(overlay(page)).toHaveClass(/active/);
    const dialog = page.locator('.settings-dialog');
    const heights: number[] = [];
    for (const tab of ['settings', 'shortcuts', 'yard', 'updates', 'about']) {
      await page.click(`.settings-tab[data-tab="${tab}"]`);
      await expect(page.locator(`.settings-pane[data-pane="${tab}"]`)).toHaveClass(/active/);
      await expect.poll(async () => (await dialog.boundingBox())?.height ?? 0).toBeGreaterThan(0);
      heights.push(Math.round((await dialog.boundingBox())!.height));
    }
    expect(new Set(heights).size, `heights per tab: ${heights}`).toBe(1);
    // Tab contents are centred, with or without the Yard attention badge,
    // and the Yard label lines up with the other tab labels.
    const centring = (hideBadge: boolean) => page.evaluate(hide => {
      const badge = document.getElementById('yardTabBadge')!;
      const wasHidden = badge.hidden;
      if (hide) badge.hidden = true; else { badge.hidden = false; badge.textContent = badge.textContent || '1'; }
      const measure = (el: HTMLElement) => {
        const style = getComputedStyle(el);
        const range = document.createRange();
        range.selectNodeContents(el);
        const content = range.getBoundingClientRect();
        const box = el.getBoundingClientRect();
        const label = document.createRange();
        label.selectNodeContents(el.firstChild!);
        const text = label.getBoundingClientRect();
        return { slack: (content.left - box.left - parseFloat(style.paddingLeft)) - (box.right - content.right - parseFloat(style.paddingRight)), mid: text.top + text.height / 2 };
      };
      const yard = measure(document.getElementById('tab-yard')!);
      const settings = measure(document.getElementById('tab-settings')!);
      badge.hidden = wasHidden;
      return { slack: Math.abs(yard.slack), drift: Math.abs(yard.mid - settings.mid) };
    }, hideBadge);
    for (const hide of [true, false]) {
      const { slack, drift } = await centring(hide);
      expect(slack, `horizontal slack with badge ${hide ? 'hidden' : 'shown'}`).toBeLessThan(2);
      expect(drift, `vertical drift with badge ${hide ? 'hidden' : 'shown'}`).toBeLessThan(1);
    }
  });

  test('gear opens the Settings tab; each tab shows its pane', async ({ page }) => {
    // The theme pill lives in the settings dialog, not the header.
    await expect(page.locator('.header .theme-pill')).toHaveCount(0);
    // The old standalone shortcuts overlay is gone (shortcuts are a tab here).
    await expect(page.locator('#shortcutsOverlay')).toHaveCount(0);

    await page.click('#settingsToggle');
    await expect(overlay(page)).toHaveClass(/active/);
    await expect(page.locator('.settings-tab.active')).toHaveText('Settings');
    const settings = page.locator('.settings-pane[data-pane="settings"]');
    await expect(settings).toHaveClass(/active/);

    // General then Review sections; General holds theme, code font and width.
    await expect(settings.locator('.settings-section-label')).toHaveText(['General', 'Review']);
    await expect(settings.locator('.settings-display-label').first()).toHaveText('Theme');
    await expect(settings.locator('.settings-display-group').nth(1).locator('.settings-display-label', { hasText: 'Hide resolved comments' })).toBeVisible();
    await expect(settings.locator('.settings-display-label').filter({ hasText: 'Code font' })).toBeVisible();
    await expect(settings.locator('.settings-display-label').filter({ hasText: 'Content Width' })).toBeVisible();
    // Subprocess agent controls stay absent.
    await expect(settings.locator('.config-card-title', { hasText: 'Agent Command' })).toHaveCount(0);

    await page.click('.settings-tab[data-tab="shortcuts"]');
    await expect(page.locator('.settings-pane[data-pane="shortcuts"]')).toHaveClass(/active/);

    await page.click('.settings-tab[data-tab="updates"]');
    const updates = page.locator('.settings-pane[data-pane="updates"]');
    await expect(updates).toHaveClass(/active/);
    await expect(updates.locator('.settings-section-label', { hasText: 'Hunkyard' })).toBeVisible();
    await expect(updates.locator('.updates-hunkyard-row')).toBeVisible();
    await expect(updates.locator('.settings-section-label', { hasText: 'AI integrations' })).toBeVisible();

    await page.click('.settings-tab[data-tab="about"]');
    const about = page.locator('.settings-pane[data-pane="about"]');
    await expect(about).toHaveClass(/active/);
    await expect(about.locator('.about-header h2')).toHaveText('Hunkyard');
    await expect(about.locator('.about-session')).toBeVisible();
    await expect(about.locator('.about-links')).toBeVisible();

    await page.click('.settings-tab[data-tab="settings"]');
    await expect(settings).toHaveClass(/active/);
  });

  test('? opens grouped Shortcuts; ?, Escape and the backdrop close the dialog', async ({ page }) => {
    await page.keyboard.press('?');
    await expect(overlay(page)).toHaveClass(/active/);
    await expect(page.locator('.settings-tab.active')).toHaveText('Shortcuts');
    // Code-review mode shows five groups in two sections: General (Navigation,
    // View) and Review (Comments, Review, Story). The Live group is filtered
    // out; Story shortcuts carry a "story mode" badge and always render.
    const pane = page.locator('.settings-pane[data-pane="shortcuts"]');
    await expect(pane.locator('.shortcuts-section-label')).toHaveText(['General', 'Review']);
    await expect(pane.locator('.shortcuts-group-label')).toHaveText(['Navigation', 'View', 'Comments', 'Review', 'Story']);

    // ? again on the Shortcuts tab closes.
    await page.keyboard.press('?');
    await expect(overlay(page)).not.toHaveClass(/active/);

    // ? on another tab switches to Shortcuts instead of closing.
    await page.click('#settingsToggle');
    await page.click('.settings-tab[data-tab="settings"]');
    await expect(page.locator('.settings-tab.active')).toHaveText('Settings');
    await page.keyboard.press('?');
    await expect(page.locator('.settings-tab.active')).toHaveText('Shortcuts');
    await expect(overlay(page)).toHaveClass(/active/);

    await page.keyboard.press('Escape');
    await expect(overlay(page)).not.toHaveClass(/active/);

    await page.click('#settingsToggle');
    await expect(overlay(page)).toHaveClass(/active/);
    // Click the overlay background (not the dialog)
    await overlay(page).click({ position: { x: 10, y: 10 } });
    await expect(overlay(page)).not.toHaveClass(/active/);
  });

  test('custom shortcut is applied immediately, persists, and can be reset', async ({ page }) => {
    await page.keyboard.press('?');
    const nextBlock = page.locator('[data-shortcut-id="next_block"]');
    await nextBlock.click();
    await page.keyboard.press('ArrowDown');
    await expect(page.locator('[data-shortcut-id="next_block"]')).toContainText('ArrowDown');

    await overlay(page).click({ position: { x: 10, y: 10 } });
    await page.keyboard.press('j');
    await expect(keyboardFocus(page)).toHaveCount(0);
    await page.keyboard.press('ArrowDown');
    await expect(keyboardFocus(page).first()).toBeVisible();

    // A write through app.js must merge with the freshly saved shortcut,
    // rather than replacing it with an older cached settings object.
    await page.keyboard.press('h');
    await page.reload();
    await expect(page.locator('.loading')).toBeHidden();
    await page.keyboard.press('?');
    await expect(page.locator('[data-shortcut-id="next_block"]')).toContainText('ArrowDown');
    await page.locator('.shortcut-reset-all').click();
    await overlay(page).click({ position: { x: 10, y: 10 } });
    await page.keyboard.press('j');
    await expect(keyboardFocus(page).first()).toBeVisible();
  });

  test('conflicting and reserved shortcuts are rejected with a toast, without changing the row height', async ({ page }) => {
    await page.keyboard.press('?');
    await page.locator('.settings-dialog').evaluate(async (element) => {
      await Promise.all(element.getAnimations().map((animation) => animation.finished));
    });
    const previousBlock = page.locator('[data-shortcut-id="previous_block"]');
    const row = previousBlock.locator('xpath=ancestor::tr');
    const heightBefore = await row.evaluate((element) => element.getBoundingClientRect().height);
    await previousBlock.click();
    const heightWhileEditing = await row.evaluate((element) => element.getBoundingClientRect().height);
    expect(heightWhileEditing).toBe(heightBefore);
    await page.keyboard.press('j');

    await expect(page.locator('.mini-toast--error')).toContainText(
      'already assigned to “Next block”',
    );
    await expect(row.locator('.shortcut-inline-feedback')).toHaveCount(0);

    // Modified submit chords stay reserved.
    const finishReview = page.locator('[data-shortcut-id="finish_review"]');
    await finishReview.click();
    await page.keyboard.press('Control+Shift+Enter');
    await expect(page.locator('.mini-toast--error').filter({ hasText: 'is reserved by Hunkyard' })).toBeVisible();
    await expect(finishReview).toContainText('Shift+F');
  });

  test('system code font overrides --hunkyard-font-code and persists across reload', async ({ page }) => {
    // theme.css's default stack (GitHub's), which the System preset leaves out.
    const DEFAULT_ONLY = 'Liberation Mono';
    const monoFont = () =>
      page.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue('--hunkyard-font-code').trim(),
      );
    const uiFont = () =>
      page.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue('--hunkyard-font-mono').trim(),
      );
    const originalUiFont = await uiFont();

    await page.click('#settingsToggle');
    expect(await monoFont()).toContain(DEFAULT_ONLY);
    await expect(page.locator('#codeFontSelect option[value="default"]')).toHaveText('Default (GitHub monospace)');

    await page.selectOption('#codeFontSelect', 'system');
    await expect(page.locator('#codeFontCustomRow')).toBeHidden();
    await expect.poll(monoFont).toBe('ui-monospace, SFMono-Regular, Menlo, Consolas, monospace');
    expect(await uiFont()).toBe(originalUiFont);
    const consumerFonts = await page.evaluate(() => {
      const probe = document.createElement('div');
      probe.innerHTML = '<div class="line-content code-line">code</div>'
        + '<div class="suggestion-diff">suggestion</div>'
        + '<div class="comment-body"><code>comment code</code></div>';
      document.body.appendChild(probe);
      return Array.from(probe.querySelectorAll('.code-line, .suggestion-diff, .comment-body code'))
        .map(element => getComputedStyle(element).fontFamily);
    });
    expect(consumerFonts).toHaveLength(3);
    consumerFonts.forEach(font => expect(font).toContain('ui-monospace'));

    await page.reload();
    await expect.poll(monoFont).toBe('ui-monospace, SFMono-Regular, Menlo, Consolas, monospace');

    // Back to Default drops the override entirely rather than pinning a copy.
    await page.click('#settingsToggle');
    await page.selectOption('#codeFontSelect', 'default');
    await expect.poll(monoFont).toContain(DEFAULT_ONLY);
  });

  test('custom code font applies, and an invalid value is rejected', async ({ page }) => {
    await page.click('#settingsToggle');
    const monoFont = () =>
      page.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue('--hunkyard-font-code').trim(),
      );

    await page.selectOption('#codeFontSelect', 'custom');
    const input = page.locator('#codeFontCustomInput');
    await expect(input).toBeVisible();
    await input.fill("'Comic Mono', monospace");
    await input.blur();
    await expect.poll(monoFont).toBe("'Comic Mono', monospace");

    // A value that could escape the declaration falls back to the default.
    await input.fill('monospace; background: red');
    await input.blur();
    await expect(page.locator('.mini-toast--error')).toContainText('valid font-family');
    await expect(input).toHaveAttribute('aria-invalid', 'true');
    await expect.poll(monoFont).toContain('Liberation Mono');
  });

  test('installed font discovery retries after a transient failure', async ({ page }) => {
    let requests = 0;
    await page.route('**/api/code-fonts', async (route) => {
      requests++;
      if (requests === 1) {
        await route.fulfill({ status: 503, body: 'Code font discovery failed' });
        return;
      }
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ code_fonts: ['Retry Mono'] }),
      });
    });

    await page.click('#settingsToggle');
    await expect.poll(() => requests).toBe(1);
    await page.click('#settingsClose');
    await page.click('#settingsToggle');
    await expect(page.locator('#codeFontSelect option', { hasText: 'Retry Mono' })).toHaveCount(1);
    expect(requests).toBe(2);
  });

  // Merged from theme.spec.ts: the theme pill sets data-theme (system removes
  // it), moves its indicator, and persists in the hunkyard-settings cookie.
  test('theme and width pills update <html>, the indicator follows, and the theme persists', async ({ page }) => {
    const html = page.locator('html');
    const indicatorLeft = () => page.locator('#settingsThemeIndicator').evaluate(el => parseFloat(el.style.left));

    await page.click('#settingsToggle');
    // System theme: no data-theme attribute, indicator at 0%.
    await expect(html).not.toHaveAttribute('data-theme', /.*/);
    await expect.poll(indicatorLeft).toBe(0);

    await page.click('[data-settings-theme="light"]');
    await expect(html).toHaveAttribute('data-theme', 'light');
    await expect.poll(indicatorLeft).toBeCloseTo(33.333, 1);

    await page.click('[data-settings-theme="dark"]');
    await expect(html).toHaveAttribute('data-theme', 'dark');
    await expect.poll(indicatorLeft).toBeCloseTo(66.666, 1);

    await page.click('[data-settings-theme="system"]');
    await expect(html).not.toHaveAttribute('data-theme', /.*/);
    await expect.poll(indicatorLeft).toBe(0);

    await page.click('[data-settings-width="compact"]');
    await expect(html).toHaveAttribute('data-width', 'compact');
    await page.click('[data-settings-width="wide"]');
    await expect(html).toHaveAttribute('data-width', 'wide');

    // An explicit theme survives a reload.
    await page.click('[data-settings-theme="dark"]');
    await expect(html).toHaveAttribute('data-theme', 'dark');
    await loadPage(page);
    await expect(html).toHaveAttribute('data-theme', 'dark');
  });
});
