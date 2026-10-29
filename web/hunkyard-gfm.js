// hunkyard-gfm.js — GitHub Flavored Markdown extensions for markdown-it.
// markdown-it covers CommonMark plus GFM tables and `~~strikethrough~~`;
// this adds the rest of what GitHub renders in repository Markdown files:
// task lists, `~single~` strikethrough, `www.` autolinks, alerts
// (`> [!NOTE]`), footnotes, math (`$x$`, `$$`, ```math), emoji shortcodes
// (`:tada:`), and the disallowed-raw-HTML tag filter.
// docs/markdown.md lists every GitHub feature and its status here.
// Dependencies: none. use(md, opts) takes the math renderer and the emoji
// table as options (window.hunkyard.math, window.hunkyard.emojiShortcodes).
(function () {
  'use strict';

  function escapeHtml(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  // ===== Task lists =====
  // `- [ ] todo` / `- [x] done`: the marker must open the item's first
  // paragraph and be followed by whitespace or the end of the line.
  var TASK_MARKER = /^\[([ xX])\](?=\s|$)\s*/;

  function taskLists(state) {
    var tokens = state.tokens;
    for (var i = 2; i < tokens.length; i++) {
      var inline = tokens[i];
      if (inline.type !== 'inline' || tokens[i - 1].type !== 'paragraph_open' ||
          tokens[i - 2].type !== 'list_item_open') continue;
      var first = inline.children[0];
      var match = first && first.type === 'text' && TASK_MARKER.exec(first.content);
      if (!match) continue;

      // The item text labels its checkbox, so screen readers announce
      // "<text>, checkbox, checked" rather than an unnamed checkbox.
      first.content = first.content.slice(match[0].length);
      var checkbox = new state.Token('html_inline', '', 0);
      checkbox.content = '<label class="task-list-item-label"><input type="checkbox" class="task-list-item-checkbox" disabled' +
        (match[1] === ' ' ? '' : ' checked') + '> ';
      var labelEnd = new state.Token('html_inline', '', 0);
      labelEnd.content = '</label>';
      inline.children.unshift(checkbox);
      inline.children.push(labelEnd);

      tokens[i - 2].attrJoin('class', 'task-list-item');
      for (var j = i - 3; j >= 0; j--) {
        if (tokens[j].level === tokens[i - 2].level - 1 &&
            (tokens[j].type === 'bullet_list_open' || tokens[j].type === 'ordered_list_open')) {
          if (!/\bcontains-task-list\b/.test(tokens[j].attrGet('class') || '')) {
            tokens[j].attrJoin('class', 'contains-task-list');
          }
          break;
        }
      }
    }
  }

  // ===== Strikethrough =====
  // GFM strikes through one or two tildes; the closer must match the opener's
  // length, and three or more tildes are literal. markdown-it only knows `~~`.
  var DOUBLE_TILDE = 0x7E;
  var SINGLE_TILDE = 0x7E + 0x10000; // distinct marker so ~ never pairs with ~~

  function strikethroughTokenize(state, silent) {
    if (silent || state.src.charCodeAt(state.pos) !== 0x7E) return false;
    var scanned = state.scanDelims(state.pos, true);
    var token = state.push('text', '', 0);
    token.content = state.src.slice(state.pos, state.pos + scanned.length);
    if (scanned.length <= 2) {
      state.delimiters.push({
        marker: scanned.length === 1 ? SINGLE_TILDE : DOUBLE_TILDE,
        length: 0,
        token: state.tokens.length - 1,
        end: -1,
        open: scanned.can_open,
        close: scanned.can_close
      });
    }
    state.pos += scanned.length;
    return true;
  }

  function strikethroughPairs(state, delimiters) {
    for (var i = 0; i < delimiters.length; i++) {
      var opener = delimiters[i];
      if ((opener.marker !== DOUBLE_TILDE && opener.marker !== SINGLE_TILDE) || opener.end === -1) continue;
      var markup = opener.marker === SINGLE_TILDE ? '~' : '~~';
      [[opener.token, 's_open', 1], [delimiters[opener.end].token, 's_close', -1]].forEach(function (edge) {
        var token = state.tokens[edge[0]];
        token.type = edge[1];
        token.tag = 'del';
        token.nesting = edge[2];
        token.markup = markup;
        token.content = '';
      });
    }
  }

  function strikethroughPostProcess(state) {
    strikethroughPairs(state, state.delimiters);
    for (var i = 0; i < state.tokens_meta.length; i++) {
      var meta = state.tokens_meta[i];
      if (meta && meta.delimiters) strikethroughPairs(state, meta.delimiters);
    }
  }

  // ===== Extended www autolinks =====
  // `www.example.com/path` links without a scheme when it starts a line or
  // follows whitespace, `*`, `_`, `~` or `(`. Trailing punctuation, an
  // unbalanced `)` and a trailing entity reference stay outside the link.
  var WWW_LINK = /(^|[\s*_~(])(www\.[\w-]+(?:\.[\w-]+)+[^\s<]*)/gi;

  function trimAutolink(url) {
    for (;;) {
      var last = url.charAt(url.length - 1);
      if (/[?!.,:*_~'"]/.test(last)) {
        url = url.slice(0, -1);
      } else if (last === ')' && url.split(')').length > url.split('(').length) {
        url = url.slice(0, -1);
      } else if (last === ';' && /&[a-z0-9]+;$/i.test(url)) {
        url = url.replace(/&[a-z0-9]+;$/i, '');
      } else {
        return url;
      }
    }
  }

  function wwwAutolinks(state) {
    state.tokens.forEach(function (block) {
      if (block.type !== 'inline') return;
      var children = block.children;
      var linkDepth = 0;
      for (var i = 0; i < children.length; i++) {
        var child = children[i];
        if (child.type === 'link_open' || child.type === 'html_inline' && /^<a[>\s]/i.test(child.content)) linkDepth++;
        if (child.type === 'link_close' || child.type === 'html_inline' && /^<\/a\s*>/i.test(child.content)) linkDepth--;
        if (child.type !== 'text' || linkDepth > 0 || !/www\./i.test(child.content)) continue;

        var text = child.content;
        var nodes = [];
        var last = 0;
        var match;
        WWW_LINK.lastIndex = 0;
        while ((match = WWW_LINK.exec(text))) {
          var start = match.index + match[1].length;
          var url = trimAutolink(match[2]);
          var href = state.md.normalizeLink('http://' + url);
          if (!url || !state.md.validateLink(href)) continue;
          if (start > last) nodes.push(textToken(state, text.slice(last, start), child.level));
          var open = new state.Token('link_open', 'a', 1);
          open.attrs = [['href', href]];
          open.markup = 'linkify';
          open.info = 'auto';
          nodes.push(open);
          nodes.push(textToken(state, state.md.normalizeLinkText(url), child.level + 1));
          var close = new state.Token('link_close', 'a', -1);
          close.markup = 'linkify';
          close.info = 'auto';
          nodes.push(close);
          last = start + url.length;
          WWW_LINK.lastIndex = last;
        }
        if (!nodes.length) continue;
        if (last < text.length) nodes.push(textToken(state, text.slice(last), child.level));
        block.children = children = children.slice(0, i).concat(nodes, children.slice(i + 1));
        i += nodes.length - 1;
      }
    });
  }

  function textToken(state, content, level) {
    var token = new state.Token('text', '', 0);
    token.content = content;
    token.level = level;
    return token;
  }

  // ===== Alerts =====
  // `> [!NOTE]` alone on a top-level blockquote's first line turns it into an
  // alert. The blockquote token keeps its type and gains meta.alert, so
  // callers that split blockquotes per child (hunkyard-line-blocks.js) can render
  // each piece through the same blockquote_open/close rules.
  var ALERT_MARKER = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$/i;

  function alerts(state) {
    var tokens = state.tokens;
    for (var i = 0; i + 2 < tokens.length; i++) {
      if (tokens[i].type !== 'blockquote_open' || tokens[i].level !== 0 ||
          tokens[i + 1].type !== 'paragraph_open' || tokens[i + 2].type !== 'inline') continue;
      var inline = tokens[i + 2];
      var children = inline.children;
      var markerEnd = 0;
      while (markerEnd < children.length && children[markerEnd].type === 'text') markerEnd++;
      var markerText = children.slice(0, markerEnd).map(function (t) { return t.content; }).join('');
      if (markerEnd === children.length || children[markerEnd].type === 'softbreak' ||
          children[markerEnd].type === 'hardbreak') {
        var match = ALERT_MARKER.exec(markerText);
        if (!match) continue;
        tokens[i].meta = { alert: match[1].toLowerCase() };
        inline.children = children.slice(markerEnd + 1);
        inline.content = inline.content.replace(/^[^\n]*\n?/, '');
        if (!inline.children.length) {
          tokens[i + 1].hidden = true;
          tokens[i + 3].hidden = true;
        }
      }
    }
  }

  // An alert is a note landmark titled by its type; the icon is CSS
  // (style.css), so sanitized comments keep it. env.gfmAlertContinued renders
  // the wrapper without the visible title, for the second and later pieces of
  // an alert split into per-line blocks: those carry the type as their label.
  function renderBlockquoteOpen(defaultRule) {
    return function (tokens, idx, options, env, self) {
      var alert = tokens[idx].meta && tokens[idx].meta.alert;
      if (!alert) return defaultRule(tokens, idx, options, env, self);
      var title = alert.charAt(0).toUpperCase() + alert.slice(1);
      var html = '<div class="markdown-alert markdown-alert-' + alert + '" role="note"';
      if (env && env.gfmAlertContinued) return html + ' aria-label="' + title + '">\n';
      return html + '>\n<p class="markdown-alert-title">' + title + '</p>\n';
    };
  }

  function renderBlockquoteClose(defaultRule) {
    return function (tokens, idx, options, env, self) {
      var open = findOpen(tokens, idx, 'blockquote_open');
      if (open && open.meta && open.meta.alert) return '</div>\n';
      return defaultRule(tokens, idx, options, env, self);
    };
  }

  function findOpen(tokens, closeIdx, openType) {
    var level = tokens[closeIdx].level;
    for (var i = closeIdx - 1; i >= 0; i--) {
      if (tokens[i].type === openType && tokens[i].level === level) return tokens[i];
    }
    return null;
  }

  // ===== Footnotes =====
  // `[^label]: text` defines a footnote (continuation lines indented four
  // spaces); `[^label]` references it and is numbered by first reference, as
  // GitHub numbers them. Definitions render where they are written, not
  // gathered at the end, so every source line keeps its own commentable block.
  function footnoteDefinition(state, startLine, endLine, silent) {
    var start = state.bMarks[startLine] + state.tShift[startLine];
    var max = state.eMarks[startLine];
    if (state.sCount[startLine] - state.blkIndent >= 4) return false;
    if (start + 4 > max || state.src.charCodeAt(start) !== 0x5B /* [ */ ||
        state.src.charCodeAt(start + 1) !== 0x5E /* ^ */) return false;

    var pos;
    for (pos = start + 2; pos < max; pos++) {
      var ch = state.src.charCodeAt(pos);
      if (ch === 0x20 || ch === 0x09) return false;
      if (ch === 0x5D /* ] */) break;
    }
    if (pos === start + 2 || pos + 1 >= max || state.src.charCodeAt(pos + 1) !== 0x3A /* : */) return false;
    if (silent) return true;

    var label = state.src.slice(start + 2, pos);
    state.env.gfmFootnotes = state.env.gfmFootnotes || {};
    state.env.gfmFootnotes[label] = true;

    var open = state.push('footnote_open', 'div', 1);
    open.meta = { label: label };
    open.map = [startLine, 0];

    // Parse the body as nested blocks indented by four, like a list item.
    pos += 2;
    var oldBMark = state.bMarks[startLine];
    var oldTShift = state.tShift[startLine];
    var oldSCount = state.sCount[startLine];
    var oldParentType = state.parentType;
    var bodyStart = pos;
    var initial = state.sCount[startLine] + pos - (state.bMarks[startLine] + state.tShift[startLine]);
    var offset = initial;
    while (pos < max) {
      var c = state.src.charCodeAt(pos);
      if (c === 0x09) offset += 4 - offset % 4;
      else if (c === 0x20) offset++;
      else break;
      pos++;
    }
    state.tShift[startLine] = pos - bodyStart;
    state.sCount[startLine] = offset - initial;
    state.bMarks[startLine] = bodyStart;
    state.blkIndent += 4;
    state.parentType = 'footnote';
    if (state.sCount[startLine] < state.blkIndent) state.sCount[startLine] += state.blkIndent;

    state.md.block.tokenize(state, startLine, endLine, true);

    state.parentType = oldParentType;
    state.blkIndent -= 4;
    state.tShift[startLine] = oldTShift;
    state.sCount[startLine] = oldSCount;
    state.bMarks[startLine] = oldBMark;

    open.map[1] = state.line;
    var close = state.push('footnote_close', 'div', -1);
    close.meta = open.meta;
    return true;
  }

  function footnoteReference(state, silent) {
    var start = state.pos;
    var max = state.posMax;
    var defined = state.env.gfmFootnotes;
    if (!defined || start + 3 >= max || state.src.charCodeAt(start) !== 0x5B ||
        state.src.charCodeAt(start + 1) !== 0x5E) return false;
    var pos;
    for (pos = start + 2; pos < max; pos++) {
      var ch = state.src.charCodeAt(pos);
      if (ch === 0x20 || ch === 0x0A) return false;
      if (ch === 0x5D) break;
    }
    if (pos === start + 2 || pos >= max) return false;
    var label = state.src.slice(start + 2, pos);
    if (!Object.prototype.hasOwnProperty.call(defined, label)) return false;
    if (!silent) state.push('footnote_ref', '', 0).meta = { label: label };
    state.pos = pos + 1;
    return true;
  }

  // Runs after inline parsing: number references in document order (refs
  // inside footnote bodies included), then hand each definition its number.
  function numberFootnotes(state) {
    var numbers = {};
    var refCounts = {};
    var next = 1;
    state.tokens.forEach(function (token) {
      (token.type === 'inline' ? token.children : []).forEach(function (child) {
        if (child.type !== 'footnote_ref') return;
        var label = child.meta.label;
        if (!numbers[label]) numbers[label] = next++;
        refCounts[label] = (refCounts[label] || 0) + 1;
        child.meta.number = numbers[label];
        child.meta.refIndex = refCounts[label];
      });
    });
    state.tokens.forEach(function (token) {
      if (token.type === 'footnote_open') token.meta.number = numbers[token.meta.label] || 0;
    });
  }

  function footnoteId(label) {
    return 'fn-' + escapeHtml(label);
  }

  function footnoteRefId(label, refIndex) {
    return 'fnref-' + escapeHtml(label) + (refIndex > 1 ? '-' + refIndex : '');
  }

  function renderFootnoteRef(tokens, idx) {
    var meta = tokens[idx].meta;
    return '<sup class="footnote-ref"><a href="#' + footnoteId(meta.label) + '" id="' +
      footnoteRefId(meta.label, meta.refIndex) + '" role="doc-noteref" aria-label="Footnote ' +
      meta.number + '">' + meta.number + '</a></sup>';
  }

  function renderFootnoteOpen(tokens, idx) {
    var meta = tokens[idx].meta;
    var marker = meta.number ? meta.number + '.' : '[^' + escapeHtml(meta.label) + ']';
    return '<div class="footnote" id="' + footnoteId(meta.label) + '" role="doc-footnote"><span class="footnote-number">' +
      marker + '</span><div class="footnote-body">\n';
  }

  function renderFootnoteClose(tokens, idx) {
    var meta = tokens[idx].meta;
    var backref = meta.number
      ? '<a class="footnote-backref" href="#' + footnoteRefId(meta.label, 1) +
        '" role="doc-backlink" aria-label="Back to reference ' + meta.number + '">\u21A9</a>'
      : '';
    return '</div>' + backref + '</div>\n';
  }

  // ===== Math =====
  // GitHub's math syntax: `$x$` and `$`x`$` inline, `$$x$$` inline in
  // display style, and `$$` blocks, which become ```math fences so that
  // hunkyard-line-blocks.js and the math fence renderer treat both forms alike.

  // `$$` opening a line, closed by `$$` at the end of the same or a later line.
  function mathBlock(state, startLine, endLine, silent) {
    var pos = state.bMarks[startLine] + state.tShift[startLine];
    var max = state.eMarks[startLine];
    if (state.sCount[startLine] - state.blkIndent >= 4 || state.src.slice(pos, pos + 2) !== '$$') return false;

    var first = state.src.slice(pos + 2, max);
    var body;
    var line = startLine;
    var sameLine = /^(.*?)\$\$\s*$/.exec(first);
    if (sameLine && sameLine[1].trim()) {
      body = sameLine[1].trim();
    } else {
      if (first.indexOf('$$') !== -1) return false;
      var lines = first.trim() ? [first.trim()] : [];
      for (line = startLine + 1; line < endLine; line++) {
        if (state.sCount[line] < state.blkIndent && state.bMarks[line] + state.tShift[line] < state.eMarks[line]) return false;
        var text = state.src.slice(state.bMarks[line] + state.tShift[line], state.eMarks[line]);
        var close = /^(.*?)\$\$\s*$/.exec(text);
        if (close) {
          if (close[1].trim()) lines.push(close[1]);
          break;
        }
        lines.push(state.src.slice(state.bMarks[line] + Math.min(state.tShift[line], state.blkIndent), state.eMarks[line]));
      }
      if (line >= endLine) return false;
      body = lines.join('\n');
    }
    if (silent) return true;

    var token = state.push('fence', 'code', 0);
    token.info = 'math';
    token.markup = '$$';
    token.content = body + '\n';
    token.map = [startLine, line + 1];
    state.line = line + 1;
    return true;
  }

  function isSpace(code) {
    return code === 0x20 || code === 0x09 || code === 0x0A;
  }

  function mathInline(state, silent) {
    var src = state.src;
    var start = state.pos;
    var max = state.posMax;
    if (src.charCodeAt(start) !== 0x24 /* $ */) return false;

    var content;
    var display = false;
    var end = -1;
    if (src.charCodeAt(start + 1) === 0x60 /* ` */) {
      // $`x`$: the code-span form, for TeX that clashes with Markdown.
      var ticks = /^`+/.exec(src.slice(start + 1, max))[0];
      var closeAt = src.indexOf(ticks + '$', start + 1 + ticks.length);
      if (closeAt === -1 || closeAt + ticks.length + 1 > max) return false;
      content = src.slice(start + 1 + ticks.length, closeAt).replace(/^ ([^]*) $/, '$1');
      end = closeAt + ticks.length + 1;
    } else {
      display = src.charCodeAt(start + 1) === 0x24;
      var open = display ? 2 : 1;
      var first = src.charCodeAt(start + open);
      if (start + open >= max || isSpace(first) || first === 0x24) return false;
      for (var pos = start + open; pos < max; pos++) {
        var ch = src.charCodeAt(pos);
        if (ch === 0x5C /* \ */) { pos++; continue; }
        if (ch !== 0x24) continue;
        if (display && src.charCodeAt(pos + 1) !== 0x24) continue;
        if (isSpace(src.charCodeAt(pos - 1))) continue;
        // Not before a word character: `$5 and $10`, `$HOME/$PATH` stay text.
        if (!display && /\w/.test(src.charAt(pos + 1))) continue;
        end = pos + open;
        break;
      }
      if (end === -1 || end > max) return false;
      content = src.slice(start + open, end - open);
    }
    if (!content.trim()) return false;
    if (!silent) {
      var token = state.push('math_inline', 'math', 0);
      token.content = content;
      token.meta = { display: display };
    }
    state.pos = end;
    return true;
  }

  // Shown until KaTeX renders it: hunkyard-math.js upgrades these in place.
  function mathPlaceholder(tex, display) {
    return '<code class="math-' + (display ? 'display' : 'inline') + '">' + escapeHtml(tex) + '</code>';
  }

  // ===== Emoji shortcodes =====
  // `:tada:` becomes 🎉 when GitHub knows the name; unknown names stay text.
  // The emoji is plain text, so screen readers announce its Unicode name.
  var SHORTCODE = /(^|[^\w:]):([a-z0-9_+-]+):(?![\w:])/gi;

  function emojiShortcodes(lookup) {
    return function (state) {
      state.tokens.forEach(function (block) {
        if (block.type !== 'inline') return;
        block.children.forEach(function (child) {
          if (child.type !== 'text' || child.content.indexOf(':') === -1) return;
          child.content = child.content.replace(SHORTCODE, function (match, before, name) {
            var emoji = lookup(name.toLowerCase());
            return emoji ? before + emoji : match;
          });
        });
      });
    };
  }

  // ===== Tables =====
  // Header cells name their column for screen readers.
  function tableHeaderScope(state) {
    state.tokens.forEach(function (token) {
      if (token.type === 'th_open') token.attrSet('scope', 'col');
    });
  }

  // ===== Disallowed raw HTML (GFM tagfilter) =====
  // These tags would change how the page itself parses or behaves, so GitHub
  // escapes their opening `<` and shows them as text.
  var DISALLOWED_TAG = /<(?=\/?(?:title|textarea|style|xmp|iframe|noembed|noframes|script|plaintext)(?=[\s/>]|$))/gi;

  function tagFilter(defaultRule) {
    return function (tokens, idx, options, env, self) {
      return defaultRule(tokens, idx, options, env, self).replace(DISALLOWED_TAG, '&lt;');
    };
  }

  function rule(md, name) {
    return md.renderer.rules[name] || function (tokens, idx, options, _env, self) {
      return self.renderToken(tokens, idx, options);
    };
  }

  // opts.footnotes (default true): false for review comments, where footnote
  //   ids from several comments would collide in one page.
  // opts.math(tex, display): HTML for a formula; default a placeholder.
  // opts.emoji(name): the emoji for a shortcode, or nothing.
  function use(md, opts) {
    opts = opts || {};
    var renderMath = opts.math || mathPlaceholder;

    md.inline.ruler.at('strikethrough', strikethroughTokenize);
    md.inline.ruler2.at('strikethrough', strikethroughPostProcess);
    md.block.ruler.before('fence', 'math_block', mathBlock, { alt: ['paragraph', 'reference', 'blockquote', 'list'] });
    md.inline.ruler.after('escape', 'math_inline', mathInline);
    if (opts.footnotes !== false) {
      md.block.ruler.before('reference', 'footnote_def', footnoteDefinition, { alt: ['paragraph', 'reference'] });
      md.inline.ruler.after('image', 'footnote_ref', footnoteReference);
    }
    md.core.ruler.after('inline', 'gfm_task_lists', taskLists);
    md.core.ruler.after('gfm_task_lists', 'gfm_alerts', alerts);
    md.core.ruler.after('gfm_alerts', 'gfm_footnote_numbers', numberFootnotes);
    md.core.ruler.after('gfm_footnote_numbers', 'gfm_table_header_scope', tableHeaderScope);
    if (opts.emoji) md.core.ruler.after('gfm_table_header_scope', 'gfm_emoji', emojiShortcodes(opts.emoji));
    if (md.options.linkify) md.core.ruler.after('linkify', 'gfm_www_autolinks', wwwAutolinks);

    var defaultFence = rule(md, 'fence');
    md.renderer.rules.fence = function (tokens, idx, options, env, self) {
      if (tokens[idx].info.trim().toLowerCase() !== 'math') return defaultFence(tokens, idx, options, env, self);
      return '<div class="math-block">' + renderMath(tokens[idx].content.replace(/\n$/, ''), true) + '</div>\n';
    };
    md.renderer.rules.math_inline = function (tokens, idx) {
      return renderMath(tokens[idx].content, tokens[idx].meta.display);
    };
    md.renderer.rules.blockquote_open = renderBlockquoteOpen(rule(md, 'blockquote_open'));
    md.renderer.rules.blockquote_close = renderBlockquoteClose(rule(md, 'blockquote_close'));
    md.renderer.rules.footnote_ref = renderFootnoteRef;
    md.renderer.rules.footnote_open = renderFootnoteOpen;
    md.renderer.rules.footnote_close = renderFootnoteClose;
    md.renderer.rules.html_block = tagFilter(rule(md, 'html_block'));
    md.renderer.rules.html_inline = tagFilter(rule(md, 'html_inline'));
    return md;
  }

  var api = { use: use };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.gfm = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
