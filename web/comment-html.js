(function (root, factory) {
  root.hunkyard = root.hunkyard || {};
  root.hunkyard.commentHtml = factory(root.DOMPurify, root);
})(typeof window !== 'undefined' ? window : globalThis, function (DOMPurify, root) {
  'use strict';

  if (!DOMPurify) throw new Error('DOMPurify must load before comment-html.js');

  // Isolated instance so comment hooks never pollute other DOMPurify users.
  var purify = typeof DOMPurify === 'function' ? DOMPurify(root) : DOMPurify;

  // Snapshot of HTMLPipeline::SanitizationFilter's public default allowlist.
  var ALLOWED_TAGS = [
    'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'br', 'b', 'i', 'strong', 'em',
    'a', 'pre', 'code', 'img', 'tt', 'div', 'ins', 'del', 'sup', 'sub',
    'p', 'picture', 'ol', 'ul', 'table', 'thead', 'tbody', 'tfoot',
    'blockquote', 'dl', 'dt', 'dd', 'kbd', 'q', 'samp', 'var', 'hr', 'ruby',
    'rt', 'rp', 'li', 'tr', 'td', 'th', 's', 'strike', 'summary', 'details',
    'caption', 'figure', 'figcaption', 'abbr', 'bdo', 'cite', 'dfn', 'mark',
    'small', 'source', 'span', 'time', 'wbr',
    // GitHub task lists; the hook below turns every input into a disabled checkbox.
    'input', 'label'
  ];
  var ALLOWED_ATTR = [
    'href', 'src', 'longdesc', 'loading', 'alt', 'itemscope', 'itemtype',
    'cite', 'srcset', 'abbr', 'accept', 'accept-charset', 'accesskey',
    'action', 'align', 'aria-describedby', 'aria-hidden', 'aria-label',
    'aria-labelledby', 'axis', 'border', 'char', 'charoff', 'charset',
    'checked', 'clear', 'cols', 'colspan', 'compact', 'coords', 'datetime',
    'dir', 'disabled', 'enctype', 'for', 'frame', 'headers', 'height',
    'hreflang', 'hspace', 'id', 'ismap', 'label', 'lang', 'maxlength',
    'media', 'method', 'multiple', 'name', 'nohref', 'noshade', 'nowrap',
    'open', 'progress', 'prompt', 'readonly', 'rel', 'rev', 'role', 'rows',
    'rowspan', 'rules', 'scope', 'selected', 'shape', 'size', 'span', 'start',
    'summary', 'tabindex', 'title', 'type', 'usemap', 'valign', 'value',
    'width', 'itemprop', 'class', 'data-ref-id'
  ];
  // Hunkyard-generated classes only — suggestion diffs + ref spans, plus the
  // GitHub Markdown classes hunkyard-gfm.js emits (task lists, alerts, math).
  // language-* survives so fenced code can be highlighted after sanitize
  // (hunkyard-code-highlight.js upgrades the mounted block); math-inline and
  // math-display placeholders are rendered by hunkyard-math.js the same way.
  var SAFE_CLASS = /^(?:language-[\w-]+|file-ref|comment-ref|comment-ref-code|suggestion(?:-[\w-]+)+|diff-word-(?:del|add)|task-list-item(?:-checkbox|-label)?|contains-task-list|markdown-alert(?:-(?:note|tip|important|warning|caution|title))?|math-(?:inline|display|block))$/;
  var SAFE_COMMENT_REF = /^(?:c|r|rp)_[a-f0-9]{6,}$/;
  var SAFE_URL = /^(?:(?:https?|mailto):|(?:\/|\.{1,2}\/|#))/i;

  function isSafeUrl(value) {
    return value === '' || SAFE_URL.test(String(value).trim());
  }

  // Split srcset candidates (comma-separated), keep only entries whose URL
  // passes the same SAFE_URL check used for src/href.
  function sanitizeSrcset(value) {
    var kept = [];
    String(value).split(',').forEach(function (entry) {
      var trimmed = entry.trim();
      if (!trimmed) return;
      var url = trimmed.split(/\s+/)[0];
      if (isSafeUrl(url)) kept.push(trimmed);
    });
    return kept.join(', ');
  }

  purify.addHook('afterSanitizeAttributes', function (node) {
    if (node.tagName === 'INPUT') {
      ['name', 'value', 'size', 'maxlength', 'readonly', 'tabindex'].forEach(function (attr) { node.removeAttribute(attr); });
      node.setAttribute('type', 'checkbox');
      node.setAttribute('disabled', '');
    }
    ['href', 'src', 'longdesc', 'cite'].forEach(function (attr) {
      if (node.hasAttribute && node.hasAttribute(attr) && !isSafeUrl(node.getAttribute(attr))) {
        node.removeAttribute(attr);
      }
    });
    if (node.hasAttribute && node.hasAttribute('srcset')) {
      var cleaned = sanitizeSrcset(node.getAttribute('srcset'));
      if (cleaned) node.setAttribute('srcset', cleaned);
      else node.removeAttribute('srcset');
    }
    if (node.tagName === 'IMG' && node.hasAttribute && !node.hasAttribute('src') && node.hasAttribute('srcset')) {
      var firstUrl = node.getAttribute('srcset').split(/\s+/)[0];
      if (isSafeUrl(firstUrl)) node.setAttribute('src', firstUrl);
    }
    if (node.hasAttribute && node.hasAttribute('class')) {
      var classes = node.getAttribute('class').split(/\s+/).filter(function (value) {
        return SAFE_CLASS.test(value);
      });
      if (classes.length) node.setAttribute('class', classes.join(' '));
      else node.removeAttribute('class');
    }
    if (node.hasAttribute && node.hasAttribute('data-ref-id') &&
      !SAFE_COMMENT_REF.test(node.getAttribute('data-ref-id'))) {
      node.removeAttribute('data-ref-id');
    }
  });

  function sanitize(html) {
    return purify.sanitize(html, {
      ALLOWED_TAGS: ALLOWED_TAGS,
      ALLOWED_ATTR: ALLOWED_ATTR,
      ALLOW_DATA_ATTR: false,
      ALLOW_ARIA_ATTR: false,
      ALLOW_UNKNOWN_PROTOCOLS: false,
      FORBID_TAGS: ['style', 'svg', 'math'],
      FORBID_ATTR: ['style']
    });
  }

  // Agents often quote review questions as **> text** instead of markdown
  // blockquotes. Normalize those lines before markdown-it runs.
  var BOLD_BLOCKQUOTE_LINE = /^\*\*\s*>\s*(.+?)\s*\*\*$/gm;

  function normalizeCommentMarkdown(src) {
    if (!src) return src;
    return String(src).replace(BOLD_BLOCKQUOTE_LINE, '> $1');
  }

  return { sanitize: sanitize, normalizeCommentMarkdown: normalizeCommentMarkdown };
});
