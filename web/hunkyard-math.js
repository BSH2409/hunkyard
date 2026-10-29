// hunkyard-math.js — math in Markdown, rendered with KaTeX like GitHub does.
// Three entry points share one lazy load of katex/katex.min.js (+ its CSS):
//   html(tex, display)  synchronous HTML for hunkyard-gfm.js; KaTeX output when
//                       loaded, else a <code class="math-*"> placeholder
//   load()              Promise of window.katex; documents await it before
//                       building their blocks when they contain math
//   upgrade(root)       replaces mounted placeholders (comments, which are
//                       sanitized and so always get placeholders)
// and a fence renderer for ```math and $$ blocks (hunkyard-gfm.js turns $$ blocks
// into math fences). KaTeX's output carries hidden MathML, so screen readers
// read the formula rather than the visual glyph soup.
// Dependencies: window.hunkyard.renderers (optional, for the fence renderer).
(function () {
  'use strict';

  function escapeHtml(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  // Untrusted input: no \href/\url/\htmlClass (trust: false), bounded macro
  // expansion, and errors render as red source text with the message in a
  // tooltip instead of throwing.
  function options(display, throwOnError) {
    return {
      displayMode: display,
      output: 'htmlAndMathml',
      throwOnError: throwOnError,
      strict: 'ignore',
      trust: false,
      maxExpand: 1000,
      maxSize: 50,
    };
  }

  function katex() {
    var k = typeof window !== 'undefined' && window.katex;
    return k && typeof k.renderToString === 'function' ? k : null;
  }

  // One shared load, started by the first document or comment with math.
  // A failed load rejects (placeholders stay as source) and is retried by
  // the next call.
  var loading = null;

  function load() {
    if (katex()) return Promise.resolve(katex());
    if (!loading) {
      loading = new Promise(function (resolve, reject) {
        if (!document.querySelector('link[data-hunkyard-katex]')) {
          var css = document.createElement('link');
          css.rel = 'stylesheet';
          css.href = 'katex/katex.min.css';
          css.setAttribute('data-hunkyard-katex', '');
          document.head.appendChild(css);
        }
        var s = document.createElement('script');
        s.src = 'katex/katex.min.js';
        s.onload = function () {
          if (katex()) resolve(katex());
          else { loading = null; reject(new Error('KaTeX is not loaded')); }
        };
        s.onerror = function () {
          loading = null;
          s.remove();
          reject(new Error('Could not load katex.min.js'));
        };
        document.head.appendChild(s);
      });
    }
    return loading;
  }

  function placeholder(tex, display) {
    return '<code class="math-' + (display ? 'display' : 'inline') + '">' + escapeHtml(tex) + '</code>';
  }

  function rendered(tex, display) {
    return '<span class="math-' + (display ? 'display' : 'inline') + '" data-hunkyard-math="rendered">' +
      katex().renderToString(tex, options(display, false)) + '</span>';
  }

  function html(tex, display) {
    if (!katex()) return placeholder(tex, display);
    try {
      return rendered(tex, display);
    } catch (e) {
      return placeholder(tex, display);
    }
  }

  var PLACEHOLDERS = 'code.math-inline:not([data-hunkyard-math]), code.math-display:not([data-hunkyard-math])';

  function upgrade(root) {
    if (!root || !root.querySelectorAll) return;
    var found = Array.prototype.slice.call(root.querySelectorAll(PLACEHOLDERS));
    if (root.matches && root.matches(PLACEHOLDERS)) found.push(root);
    if (!found.length) return;
    found.forEach(function (code) { code.setAttribute('data-hunkyard-math', 'pending'); });
    load().then(function () {
      found.forEach(function (code) {
        if (!code.isConnected) return;
        var display = code.classList.contains('math-display');
        try {
          code.outerHTML = rendered(code.textContent, display);
        } catch (e) {
          code.setAttribute('data-hunkyard-math', 'error');
        }
      });
    }, function () {
      found.forEach(function (code) { code.setAttribute('data-hunkyard-math', 'error'); });
    });
  }

  // Does a parsed document need KaTeX? Math fences render through the fence
  // renderer on their own; inline math renders while blocks are built.
  function needed(tokens) {
    return tokens.some(function (t) {
      return t.type === 'inline' && t.children && t.children.some(function (c) {
        return c.type === 'math_inline';
      });
    });
  }

  // ```math fences and $$ blocks. A parse error rejects, so the registry
  // shows the source with KaTeX's message.
  function renderFence(source, ctx) {
    return load().then(function (k) {
      ctx.container.innerHTML = '<div class="math-display" data-hunkyard-math="rendered">' +
        k.renderToString(source, options(true, true)) + '</div>';
    });
  }

  var fence = { kind: 'fence', name: 'math', langs: ['math'], render: renderFence };

  if (typeof window !== 'undefined' && window.hunkyard && window.hunkyard.renderers) {
    window.hunkyard.renderers.register(fence);
  }

  var api = { html: html, load: load, upgrade: upgrade, needed: needed, fence: fence };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.math = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
