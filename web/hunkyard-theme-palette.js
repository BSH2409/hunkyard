(function() {
  'use strict';
  const DEFAULTS = { dark: 'github-dark-default', light: 'github-light-default' };

  function pair(settings, palettes) {
    const result = {};
    ['light', 'dark'].forEach(function(mode) {
      const requested = settings[mode + 'Palette'];
      const match = palettes.find(function(p) { return p.id === requested && p.type === mode; });
      result[mode] = match ? match.id : DEFAULTS[mode];
    });
    return result;
  }

  function apply(settings, palettes, root, systemLight) {
    const themes = pair(settings, palettes);
    const mode = settings.theme === 'light' || settings.theme === 'dark' ? settings.theme : systemLight ? 'light' : 'dark';
    const palette = palettes.find(function(p) { return p.id === themes[mode]; });
    if (!palette) return;
    root.dataset.hunkyardPalette = palette.id;
    root.style.colorScheme = mode;
    Object.keys(palette.colors).forEach(function(role) {
      root.style.setProperty('--hunkyard-palette-' + role, palette.colors[role]);
    });
    return palette;
  }

  // The hunkyard-settings cookie (JSON), read directly: pages call applySaved()
  // from <head>, before hunkyard-shared.js has loaded.
  function savedSettings() {
    const match = typeof document !== 'undefined' && document.cookie.match(/(?:^|;\s*)hunkyard-settings=([^;]*)/);
    if (!match) return {};
    try { return JSON.parse(decodeURIComponent(match[1])) || {}; } catch (_) { return {}; }
  }

  // Theme <html> with the saved palette (window.hunkyard.palettes, from
  // pierre/palettes.js). Safe to call again after a theme or palette change.
  function applySaved(root) {
    const palettes = window.hunkyard && window.hunkyard.palettes;
    if (!palettes) return;
    const systemLight = !!(window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches);
    return apply(savedSettings(), palettes, root || document.documentElement, systemLight);
  }

  // "System" follows the OS; re-theme when it flips. Live mode needs this
  // (index.html without app.js); on the review page app.js re-applies too.
  if (typeof window !== 'undefined' && window.matchMedia) {
    window.matchMedia('(prefers-color-scheme: light)').addEventListener('change', function() {
      const theme = savedSettings().theme;
      // The /themes page shows a chosen palette, not the saved one.
      if (document.documentElement.hasAttribute('data-hunkyard-theme-preview')) return;
      if (theme !== 'light' && theme !== 'dark') applySaved();
    });
  }

  const api = { pair: pair, apply: apply, applySaved: applySaved, DEFAULTS: DEFAULTS };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.themePalette = api;
  }
  if (typeof module === 'object' && module.exports) module.exports = api;
})();
