const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');
const markdownit = require('markdown-it');
const gfm = require('../hunkyard-gfm.js');

// Same options as app.js documentMd (minus Shiki highlighting).
function makeMd(opts) {
  return gfm.use(markdownit({ html: true, linkify: true }), opts);
}

function render(src, opts) {
  return makeMd(opts).render(src);
}

// hunkyard-line-blocks.js in the fake-browser shim used by hunkyard-line-blocks.test.js.
const lineBlocksSrc = fs.readFileSync(path.join(__dirname, '..', 'hunkyard-line-blocks.js'), 'utf8');
const sandboxWindow = { hunkyard: {} };
new Function('window', 'document', lineBlocksSrc)(sandboxWindow, {});
const { buildLineBlocks } = sandboxWindow.hunkyard.lineBlocks;

function blocksFor(src) {
  const md = makeMd();
  return buildLineBlocks(md.parse(src, {}), md, src);
}

// --- Task lists ---

test('task list items render disabled checkboxes and GitHub classes', () => {
  const html = render('- [ ] todo\n- [x] done\n- [X] shouted');
  assert.match(html, /<ul class="contains-task-list">/);
  assert.match(html, /<li class="task-list-item"><label class="task-list-item-label"><input type="checkbox" class="task-list-item-checkbox" disabled> todo<\/label><\/li>/);
  assert.match(html, /<input type="checkbox" class="task-list-item-checkbox" disabled checked> done/);
  assert.match(html, /disabled checked> shouted/);
});

test('task markers need trailing whitespace and must open the item', () => {
  const html = render('- [ ]text\n- see [x] later');
  assert.doesNotMatch(html, /checkbox/);
  assert.match(html, /\[ \]text/);
});

test('task lists work in nested and loose lists', () => {
  const html = render('1. parent\n\n   - [x] child\n\n2. [ ] loose');
  assert.match(html, /<li class="task-list-item"><label[^>]*><input[^>]+checked> child<\/label>/);
  assert.match(html, /<li class="task-list-item">\n<p><label[^>]*><input[^>]+disabled> loose<\/label><\/p>/);
});

test('task list items keep their class in per-item line blocks', () => {
  const blocks = blocksFor('- [ ] one\n- [x] two\n');
  assert.equal(blocks[0].startLine, 1);
  assert.match(blocks[0].html, /<li class="task-list-item"><label[^>]*><input[^>]+disabled> one<\/label>/);
  assert.match(blocks[1].html, /<li class="task-list-item"><label[^>]*><input[^>]+checked> two<\/label>/);
});

// --- Strikethrough ---

test('one or two tildes strike through; the closer must match', () => {
  assert.equal(render('~~two~~ ~one~'), '<p><del>two</del> <del>one</del></p>\n');
  assert.equal(render('a ~~~three~~~'), '<p>a ~~~three~~~</p>\n');
  assert.equal(render('~mixed~~'), '<p>~mixed~~</p>\n');
});

test('strikethrough nests with emphasis', () => {
  assert.equal(render('~**bold**~'), '<p><del><strong>bold</strong></del></p>\n');
});

// --- Autolinks ---

test('www. autolinks get an http:// href and trim trailing punctuation', () => {
  assert.equal(
    render('Visit www.commonmark.org/help.'),
    '<p>Visit <a href="http://www.commonmark.org/help">www.commonmark.org/help</a>.</p>\n'
  );
  assert.match(render('(www.example.com)'), /\(<a href="http:\/\/www\.example\.com">www\.example\.com<\/a>\)/);
  assert.match(render('www.google.com/search?q=(business))+ok'), /href="http:\/\/www\.google\.com\/search\?q=\(business\)\)\+ok"/);
  assert.match(render('www.example.com/a_(b))'), /href="http:\/\/www\.example\.com\/a_\(b\)">[^<]+<\/a>\)/);
});

test('www. needs a word boundary and is not linked twice', () => {
  assert.doesNotMatch(render('notwww.example.com'), /<a/);
  assert.equal((render('https://www.example.com').match(/<a /g) || []).length, 1);
  assert.equal((render('[www.example.com](https://x.test)').match(/<a /g) || []).length, 1);
  assert.equal((render('<a href="/x">www.example.com</a>').match(/<a /g) || []).length, 1);
  assert.doesNotMatch(render('`www.example.com`'), /<a/);
});

test('scheme and email autolinks still work', () => {
  assert.match(render('https://example.com'), /<a href="https:\/\/example\.com">/);
  assert.match(render('foo@bar.com'), /<a href="mailto:foo@bar\.com">/);
});

// --- Alerts ---

test('alerts render GitHub markup for every type', () => {
  for (const type of ['NOTE', 'TIP', 'IMPORTANT', 'WARNING', 'CAUTION']) {
    const lower = type.toLowerCase();
    const html = render('> [!' + type + ']\n> Body text.');
    assert.match(html, new RegExp('<div class="markdown-alert markdown-alert-' + lower + '" role="note">'));
    assert.match(html, new RegExp('<p class="markdown-alert-title">' + type.charAt(0) + lower.slice(1) + '</p>'));
    assert.match(html, /<p>Body text\.<\/p>\n<\/div>/);
    assert.doesNotMatch(html, /blockquote|\[!/);
  }
});

test('alert markers are case-insensitive and must stand alone on the first line', () => {
  assert.match(render('> [!tip]\n> x'), /markdown-alert-tip/);
  assert.doesNotMatch(render('> [!TIP] inline\n> x'), /markdown-alert/);
  assert.doesNotMatch(render('> text\n> [!TIP]'), /markdown-alert/);
  assert.doesNotMatch(render('> [!UNKNOWN]\n> x'), /markdown-alert/);
});

test('alerts only apply to top-level blockquotes', () => {
  assert.doesNotMatch(render('- > [!NOTE]\n  > nested'), /markdown-alert/);
  assert.doesNotMatch(render('> > [!NOTE]\n> > nested'), /markdown-alert/);
});

test('an alert marker on its own paragraph leaves no empty paragraph', () => {
  const html = render('> [!NOTE]\n>\n> Body.');
  assert.doesNotMatch(html, /<p><\/p>/);
  assert.match(html, /Note<\/p>\n+<p>Body\.<\/p>/);
});

test('split alert line blocks keep the alert wrapper and title once', () => {
  const blocks = blocksFor('> [!WARNING]\n> First.\n>\n> Second.\n');
  const withContent = blocks.filter(b => /markdown-alert/.test(b.html));
  assert.equal(withContent.length, 3);
  assert.equal(withContent[1].startLine, 3);
  assert.equal(withContent[1].html, '<div class="markdown-alert markdown-alert-warning" role="note" aria-label="Warning">\n</div>\n');
  withContent.splice(1, 1);
  assert.match(withContent[0].html, /markdown-alert-title[^]*First\./);
  assert.doesNotMatch(withContent[1].html, /markdown-alert-title/);
  assert.match(withContent[1].html, /<div class="markdown-alert markdown-alert-warning" role="note" aria-label="Warning">\n<p>Second\.<\/p>\n<\/div>/);
  assert.equal(withContent[1].startLine, 4);
});

test('plain blockquotes still split into blockquote line blocks', () => {
  const blocks = blocksFor('> one\n>\n> two\n');
  assert.equal(blocks[0].html, '<blockquote>\n<p>one</p>\n</blockquote>\n');
  assert.equal(blocks[2].html, '<blockquote>\n<p>two</p>\n</blockquote>\n');
});

// --- Footnotes ---

test('footnote references number by first use and link both ways', () => {
  const html = render('A[^z] B[^a] C[^z]\n\n[^a]: Alpha.\n[^z]: Zed.\n');
  const ref = (label, id, n) => '<sup class="footnote-ref"><a href="#fn-' + label + '" id="' + id +
    '" role="doc-noteref" aria-label="Footnote ' + n + '">' + n + '</a></sup>';
  assert.ok(html.includes('A' + ref('z', 'fnref-z', 1)));
  assert.ok(html.includes('B' + ref('a', 'fnref-a', 2)));
  assert.ok(html.includes('C' + ref('z', 'fnref-z-2', 1)));
  assert.match(html, /<div class="footnote" id="fn-a" role="doc-footnote"><span class="footnote-number">2\.<\/span><div class="footnote-body">\n<p>Alpha\.<\/p>\n<\/div><a class="footnote-backref" href="#fnref-a" role="doc-backlink" aria-label="Back to reference 2">/);
  assert.match(html, /id="fn-z" role="doc-footnote"><span class="footnote-number">1\.<\/span>/);
});

test('undefined footnote references stay literal', () => {
  assert.equal(render('See [^nope].'), '<p>See [^nope].</p>\n');
});

test('footnote bodies take indented continuation blocks', () => {
  const html = render('x[^n]\n\n[^n]: First para\n    lazy line.\n\n    Second para.\n\nAfter.\n');
  assert.match(html, /<p>First para\nlazy line\.<\/p>\n<p>Second para\.<\/p>\n<\/div>/);
  assert.match(html, /<\/div>\n<p>After\.<\/p>/);
});

test('unreferenced footnotes render with their label', () => {
  assert.match(render('[^orphan]: Text.'), /<span class="footnote-number">\[\^orphan\]<\/span>/);
});

test('footnote definitions are single line blocks at their source lines', () => {
  const blocks = blocksFor('Ref[^1].\n\n[^1]: Note\n    more.\n');
  const def = blocks.find(b => /class="footnote"/.test(b.html));
  assert.equal(def.startLine, 3);
  assert.equal(def.endLine, 4);
  assert.match(def.html, /footnote-number">1\./);
});

test('footnotes can be turned off (review comments)', () => {
  // Plain CommonMark then: the definition is a link reference.
  assert.doesNotMatch(render('x[^1]\n\n[^1]: y', { footnotes: false }), /footnote/);
});

// --- Math ---

const fakeMath = (tex, display) => '<M' + (display ? ' display' : '') + '>' + tex + '</M>';

test('inline math: $x$, $`x`$ and $$x$$', () => {
  assert.equal(render('a $x^2$ b', { math: fakeMath }), '<p>a <M>x^2</M> b</p>\n');
  assert.equal(render('$`a $b$ c`$', { math: fakeMath }), '<p><M>a $b$ c</M></p>\n');
  assert.equal(render('so $$\\sum_i$$ ok', { math: fakeMath }), '<p>so <M display>\\sum_i</M> ok</p>\n');
  assert.equal(render('$\\frac{a}{b}$', { math: fakeMath }), '<p><M>\\frac{a}{b}</M></p>\n');
});

test('inline math keeps markdown out of TeX', () => {
  assert.equal(render('$a_1 * b_2 * c$', { math: fakeMath }), '<p><M>a_1 * b_2 * c</M></p>\n');
});

test('dollar signs in prose stay text', () => {
  for (const src of ['costs $5 and $10', '$HOME/$PATH', '$ x$', '$x $', 'a $$ b', '\\$x$', '`$x$`']) {
    assert.doesNotMatch(render(src, { math: fakeMath }), /<M/, src);
  }
});

test('$$ blocks and ```math fences render as display math', () => {
  assert.equal(render('$$\nx = 1\n\\\\ y\n$$\n\nafter', { math: fakeMath }),
    '<div class="math-block"><M display>x = 1\n\\\\ y</M></div>\n<p>after</p>\n');
  assert.equal(render('$$ x $$', { math: fakeMath }), '<div class="math-block"><M display>x</M></div>\n');
  assert.equal(render('```math\nz\n```', { math: fakeMath }), '<div class="math-block"><M display>z</M></div>\n');
  assert.equal(render('para\n$$\nx\n$$', { math: fakeMath }), '<p>para</p>\n<div class="math-block"><M display>x</M></div>\n');
  assert.match(render('$$\nunclosed', { math: fakeMath }), /<p>\$\$\nunclosed<\/p>/);
});

test('math defaults to escaped placeholders hunkyard-math.js upgrades', () => {
  assert.equal(render('$a<b$'), '<p><code class="math-inline">a&lt;b</code></p>\n');
  assert.equal(render('$$\nx\n$$'), '<div class="math-block"><code class="math-display">x</code></div>\n');
});

test('$$ blocks become math fences with source maps for line blocks', () => {
  const tokens = makeMd().parse('Intro\n\n$$\na\nb\n$$\n', {});
  const fence = tokens.find(t => t.type === 'fence');
  assert.equal(fence.info, 'math');
  assert.equal(fence.content, 'a\nb\n');
  assert.deepEqual(fence.map, [2, 6]);
});

// --- Emoji ---

const EMOJI = { tada: '🎉', '+1': '👍', '100': '💯' };
const emoji = name => EMOJI[name];

test('known shortcodes become emoji, unknown ones stay text', () => {
  assert.equal(render(':tada: :+1: :100: :nope:', { emoji }), '<p>🎉 👍 💯 :nope:</p>\n');
  assert.equal(render('(:TADA:)', { emoji }), '<p>(🎉)</p>\n');
});

test('shortcodes need boundaries and skip code', () => {
  assert.equal(render('a:tada: 10:100:20 :tada:b', { emoji }), '<p>a:tada: 10:100:20 :tada:b</p>\n');
  assert.equal(render('`:tada:`', { emoji }), '<p><code>:tada:</code></p>\n');
  assert.equal(render(':tada:', {}), '<p>:tada:</p>\n');
});

// --- Accessibility ---

test('table header cells carry scope="col"', () => {
  assert.match(render('| a | b |\n|---|---|\n| 1 | 2 |'), /<th scope="col">a<\/th>\n<th scope="col">b<\/th>/);
});

// --- Disallowed raw HTML ---

test('GFM tagfilter escapes disallowed tags in block and inline HTML', () => {
  const html = render('<script>alert(1)</script>\n\ntext <style>p{}</style> <iframe src="x"></iframe> <b>ok</b>');
  for (const tag of ['script', 'style', 'iframe']) {
    assert.doesNotMatch(html, new RegExp('<' + tag));
    assert.match(html, new RegExp('&lt;' + tag));
    assert.match(html, new RegExp('&lt;/' + tag));
  }
  assert.match(html, /<b>ok<\/b>/);
  for (const tag of ['title', 'textarea', 'xmp', 'noembed', 'noframes', 'plaintext']) {
    assert.match(render('<' + tag + '>x</' + tag + '>'), new RegExp('&lt;' + tag + '>x&lt;/' + tag + '>'));
  }
  assert.match(render('<details><summary>S</summary>\n\nbody\n\n</details>'), /<details><summary>S<\/summary>/);
  assert.match(render('<scripts>x'), /<scripts>/);
});

// --- Core GFM features markdown-it already provides ---

test('tables, fenced code and hard breaks keep working', () => {
  assert.match(render('| a | b |\n|:--|--:|\n| `x\\|y` | z |'), /<td style="text-align:left"><code>x\|y<\/code><\/td>/);
  assert.match(render('```js\nlet a\n```'), /<pre><code class="language-js">let a\n<\/code><\/pre>/);
  assert.match(render('a\\\nb  \nc'), /a<br>\nb<br>\nc/);
});
