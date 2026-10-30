const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');

// --- Sandbox setup ---
// We need window.hunkyard.shared.getCookie/setCookie available before loading
// hunkyard-comment-templates.js. Build a minimal shim.

function makeSandbox() {
  var cookieJar = '';
  var doc = {
    get cookie() { return cookieJar; },
    set cookie(v) { cookieJar = v; },
    createElement: function (tag) {
      var el = {
        tagName: tag.toUpperCase(),
        className: '',
        textContent: '',
        title: '',
        style: {},
        innerHTML: '',
        children: [],
        _listeners: {},
        appendChild: function (child) { el.children.push(child); return child; },
        addEventListener: function (evt, fn) {
          if (!el._listeners[evt]) el._listeners[evt] = [];
          el._listeners[evt].push(fn);
        },
      };
      return el;
    },
  };
  var win = { hunkyard: { shared: {} } };

  // Load hunkyard-shared.js to get real getCookie/setCookie.
  var sharedSrc = fs.readFileSync(path.join(__dirname, '..', 'hunkyard-shared.js'), 'utf8');
  var sharedFn = new Function('window', 'document', sharedSrc);
  sharedFn(win, doc);

  // Now load the module under test.
  var src = fs.readFileSync(path.join(__dirname, '..', 'hunkyard-comment-templates.js'), 'utf8');
  var fn = new Function('window', 'document', 'module', src + '\nreturn window;');
  var mod = { exports: {} };
  fn(win, doc, mod);

  return { win: win, doc: doc, mod: mod, setCookieRaw: function (v) { cookieJar = v; } };
}

// --- Tests ---

test('getTemplates returns empty array when no cookie', () => {
  var sb = makeSandbox();
  var api = sb.win.hunkyard.commentTemplates;
  assert.deepEqual(api.getTemplates(), []);
});

test('saveTemplates + getTemplates roundtrip', () => {
  var sb = makeSandbox();
  var api = sb.win.hunkyard.commentTemplates;
  var templates = [{ title: 'Typo', body: 'Fix typo' }, { title: '', body: 'LGTM' }];
  api.saveTemplates(templates);
  // Simulate browser echoing cookie back (only name=value, no attributes).
  var raw = sb.doc.cookie.split(';')[0]; // "hunkyard-templates=..."
  sb.setCookieRaw(raw);
  assert.deepEqual(api.getTemplates(), templates);
});

test('getTemplates reads legacy string templates as untitled and drops malformed entries', () => {
  var sb = makeSandbox();
  var api = sb.win.hunkyard.commentTemplates;
  sb.setCookieRaw('hunkyard-templates=' + encodeURIComponent(JSON.stringify(['Fix typo', 42, { title: 'x' }])));
  assert.deepEqual(api.getTemplates(), [{ title: '', body: 'Fix typo' }]);
});

test('chips show the title, fall back to the body, and insert the body', () => {
  var sb = makeSandbox();
  var api = sb.win.hunkyard.commentTemplates;
  api.saveTemplates([{ title: 'Needs test', body: 'Please add a test covering this branch.' }, { title: '', body: 'LGTM' }]);
  sb.setCookieRaw(sb.doc.cookie.split(';')[0]);

  var inserted = [];
  var bar = api.buildTemplateBar({ onInsert: function (text) { inserted.push(text); } });
  var titled = bar.children[0];
  assert.equal(titled.children[0].textContent, 'Needs test');
  assert.equal(titled.title, 'Please add a test covering this branch.');
  assert.equal(bar.children[1].children[0].textContent, 'LGTM');

  titled._listeners.click[0]({ preventDefault: function () {} });
  assert.deepEqual(inserted, ['Please add a test covering this branch.']);
});

test('_saveNew stores the title alongside the body', () => {
  var sb = makeSandbox();
  var api = sb.win.hunkyard.commentTemplates;
  var saved = [];
  var bar = api.buildTemplateBar({ onSaveNew: function (tmpl) { saved.push(tmpl); } });
  bar._saveNew('Nit', 'Minor nit, feel free to ignore.');
  sb.setCookieRaw(sb.doc.cookie.split(';')[0]);
  assert.deepEqual(api.getTemplates(), [{ title: 'Nit', body: 'Minor nit, feel free to ignore.' }]);
  assert.deepEqual(saved, [{ title: 'Nit', body: 'Minor nit, feel free to ignore.' }]);
});

test('buildTemplateBar returns a DOM element with correct class', () => {
  var sb = makeSandbox();
  var api = sb.win.hunkyard.commentTemplates;

  // Seed one template so the bar renders chips.
  api.saveTemplates([{ title: '', body: 'Nice!' }]);
  var raw = sb.doc.cookie.split(';')[0];
  sb.setCookieRaw(raw);

  var inserted = [];
  var bar = api.buildTemplateBar({
    onInsert: function (text) { inserted.push(text); },
    onSaveNew: function () {},
  });
  assert.equal(bar.className, 'comment-template-bar');
  // Should have one chip child.
  assert.equal(bar.children.length, 1);
  assert.equal(bar.children[0].className, 'template-chip');
});

test('buildTemplateBar hides when no templates', () => {
  var sb = makeSandbox();
  var api = sb.win.hunkyard.commentTemplates;
  var bar = api.buildTemplateBar({ onInsert: function () {}, onSaveNew: function () {} });
  assert.equal(bar.style.display, 'none');
});

test('CommonJS module.exports matches window.hunkyard.commentTemplates', () => {
  var sb = makeSandbox();
  assert.strictEqual(sb.mod.exports, sb.win.hunkyard.commentTemplates);
});
