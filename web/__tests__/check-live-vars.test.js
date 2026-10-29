const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const css = fs.readFileSync(path.join(__dirname, '..', 'theme.css'), 'utf8');

// R2 + Phase C + Phase D tokens.
const REQUIRED_VARS = [
  '--hunkyard-live-iframe-frame',
  '--hunkyard-live-iframe-bg',
  '--hunkyard-live-composer-border',
  '--hunkyard-live-composer-error-fg',
  '--hunkyard-live-ancestor-menu-bg',
  '--hunkyard-live-ancestor-menu-fg',
  '--hunkyard-live-ancestor-menu-hover-bg',
  // Phase D markers + re-anchor
  '--hunkyard-live-marker-bg',
  '--hunkyard-live-marker-fg',
  '--hunkyard-live-marker-border',
  '--hunkyard-live-marker-shadow',
  '--hunkyard-live-marker-focus-ring',
  '--hunkyard-live-reanchor-active-outline',
];

const BLOCKS = [
  { name: ':root',                              re: /:root\s*\{([\s\S]*?)\n\}/m },
  { name: 'prefers-color-scheme: light',        re: /@media \(prefers-color-scheme: light\)\s*\{[\s\S]*?html:not\(\[data-theme\]\)\s*\{([\s\S]*?)\n\s*\}/ },
  { name: '[data-theme="dark"]',                re: /\[data-theme="dark"\]\s*\{([\s\S]*?)\n\}/ },
  { name: '[data-theme="light"]',               re: /\[data-theme="light"\]\s*\{([\s\S]*?)\n\}/ },
];

for (const b of BLOCKS) {
  test(`block ${b.name} defines all --hunkyard-live-* vars`, () => {
    const m = b.re.exec(css);
    assert.ok(m, 'block not found: ' + b.name);
    const body = m[1];
    for (const v of REQUIRED_VARS) {
      assert.match(body, new RegExp(v + '\\s*:'), `missing ${v} in ${b.name}`);
    }
  });
}
