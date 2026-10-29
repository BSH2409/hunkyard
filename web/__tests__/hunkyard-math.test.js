const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');
const markdownit = require('markdown-it');
const katex = require('katex');
const gfm = require('../hunkyard-gfm.js');

// Load hunkyard-math.js with a fake window whose renderer registry records the
// fence registration. window.katex is set per test.
const src = fs.readFileSync(path.join(__dirname, '..', 'hunkyard-math.js'), 'utf8');
const registered = [];
const sandboxWindow = { hunkyard: { renderers: { register: r => registered.push(r) } } };
new Function('window', src)(sandboxWindow);
const math = sandboxWindow.hunkyard.math;

function withKatex(fn) {
  sandboxWindow.katex = katex;
  try { return fn(); } finally { delete sandboxWindow.katex; }
}

test('registers a fence renderer for ```math', () => {
  assert.equal(registered.length, 1);
  assert.equal(registered[0].kind, 'fence');
  assert.deepEqual(registered[0].langs, ['math']);
});

test('html() is an escaped placeholder until KaTeX loads', () => {
  assert.equal(math.html('a<b', false), '<code class="math-inline">a&lt;b</code>');
  assert.equal(math.html('x', true), '<code class="math-display">x</code>');
});

test('html() renders KaTeX with MathML for assistive technology', () => {
  withKatex(() => {
    const html = math.html('\\frac{a}{b}', false);
    assert.match(html, /^<span class="math-inline" data-hunkyard-math="rendered">/);
    assert.match(html, /<math xmlns="http:\/\/www\.w3\.org\/1998\/Math\/MathML">/);
    assert.match(html, /<annotation encoding="application\/x-tex">\\frac\{a\}\{b\}<\/annotation>/);
    assert.match(html, /class="katex-html" aria-hidden="true"/);
    assert.match(math.html('x', true), /class="katex-display"/);
  });
});

test('html() shows TeX errors in place instead of throwing', () => {
  withKatex(() => {
    assert.match(math.html('\\frac{', false), /class="katex-error"[^>]*title="ParseError/);
  });
});

test('html() does not trust links or HTML extensions in TeX', () => {
  withKatex(() => {
    const html = math.html('\\href{javascript:alert(1)}{x} \\htmlClass{evil}{y}', false);
    assert.doesNotMatch(html, /<a |href=|class="evil"/);
  });
});

test('needed() spots inline math only', () => {
  const md = gfm.use(markdownit());
  assert.equal(math.needed(md.parse('plain $5 text', {})), false);
  assert.equal(math.needed(md.parse('$$\nx\n$$', {})), false);
  assert.equal(math.needed(md.parse('see $x$', {})), true);
  assert.equal(math.needed(md.parse('- item $y$', {})), true);
});

test('the fence renderer renders display math into its container', async () => {
  sandboxWindow.katex = katex;
  try {
    const ctx = { container: { innerHTML: '' } };
    await registered[0].render('x^2', ctx);
    assert.match(ctx.container.innerHTML, /^<div class="math-display" data-hunkyard-math="rendered"><span class="katex-display">/);
    await assert.rejects(registered[0].render('\\frac{', { container: {} }), /ParseError/);
  } finally {
    delete sandboxWindow.katex;
  }
});
