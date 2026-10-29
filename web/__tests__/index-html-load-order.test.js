// Regression: app.js calls window.hunkyard.shared.installSidebarResize(...) at
// init time (and uses other window.hunkyard.shared.* helpers). hunkyard-shared.js
// must be loaded AND parsed before app.js executes, otherwise:
//   TypeError: Cannot read properties of undefined (reading 'installSidebarResize')
//
// Two requirements for the non-live (code-review) branch in index.html:
//   1. hunkyard-shared.js must actually be loaded (it's a dependency of app.js).
//   2. Dynamically-inserted <script> tags must set async=false to preserve
//      DOM execution order (the live branch already does this).

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');

const html = fs.readFileSync(path.join(__dirname, '..', 'index.html'), 'utf8');

function getElseBlock() {
  const elseMatch = html.match(/} else \{([\s\S]*?)\}\s*\)\(\)\;/);
  assert.ok(elseMatch, 'expected to find the non-live else { ... } branch');
  return elseMatch[1];
}

test('index.html non-live branch loads hunkyard-shared.js (app.js depends on window.hunkyard.shared)', () => {
  const elseBlock = getElseBlock();
  assert.match(
    elseBlock,
    /\.src\s*=\s*['"]hunkyard-shared\.js['"]/,
    'hunkyard-shared.js must be loaded in the non-live branch — app.js calls ' +
    'window.hunkyard.shared.installSidebarResize and other shared helpers at init'
  );
});

test('index.html non-live branch sets async=false on dynamically inserted scripts', () => {
  const elseBlock = getElseBlock();

  // Find every `var X = document.createElement('script')` declaration in the
  // else block and assert each one is followed by an `X.async = false`.
  // Without this, dynamically-inserted scripts default to async=true and may
  // execute out of DOM order.
  const scriptDecls = [...elseBlock.matchAll(
    /var (\w+) = document\.createElement\('script'\);/g
  )].map(m => m[1]);

  assert.ok(scriptDecls.length > 0, 'expected at least one script tag in the non-live branch');

  for (const name of scriptDecls) {
    // Module scripts (the @pierre/diffs bundle) never join async=false
    // ordering; app.js awaits window.hunkyardPierreReady instead.
    if (new RegExp(`${name}\\.type\\s*=\\s*'module'`).test(elseBlock)) continue;
    const asyncRe = new RegExp(`${name}\\.async\\s*=\\s*false`);
    assert.match(
      elseBlock,
      asyncRe,
      `script var "${name}" must set ${name}.async = false to preserve execution order`
    );
  }
});

test('index.html non-live branch loads the Pierre bundle as a module and exposes a readiness promise', () => {
  const elseBlock = getElseBlock();
  assert.match(elseBlock, /window\.hunkyardPierreReady\s*=\s*new Promise/, 'app.js awaits window.hunkyardPierreReady before rendering');
  assert.match(elseBlock, /\.type\s*=\s*'module';\s*\w+\.src\s*=\s*'pierre\/pierre-diffs\.js'/, 'Pierre bundle loads as an ES module');
  const adapter = elseBlock.indexOf("'hunkyard-pierre-adapter.js'");
  const runtime = elseBlock.indexOf("'hunkyard-pierre-runtime.js'");
  const dom = elseBlock.indexOf("'hunkyard-pierre-dom.js'");
  const view = elseBlock.indexOf("'hunkyard-pierre-view.js'");
  const app = elseBlock.indexOf("'app.js'");
  assert.ok(adapter > 0 && view > adapter && app > view, 'adapter → view → app.js order');
  for (const [name, position] of [['runtime', runtime], ['DOM', dom]]) {
    assert.ok(position > 0 && app > position,
      name + ' must load before app.js');
  }
});

test('review and theme preview apply saved palettes synchronously before first paint', () => {
  for (const filename of ['index.html', 'themes.html']) {
    const source = fs.readFileSync(path.join(__dirname, '..', filename), 'utf8');
    const head = source.match(/<head>([\s\S]*?)<\/head>/)[1];
    const palettes = head.indexOf('<script src="pierre/palettes.js"></script>');
    const paletteRuntime = head.indexOf('<script src="hunkyard-theme-palette.js"></script>');
    const applySaved = head.indexOf('themePalette.applySaved()');
    assert.ok(palettes >= 0 && paletteRuntime > palettes && applySaved > paletteRuntime,
      filename + ' must load palettes and runtime before applying the saved theme in head');
  }
});
