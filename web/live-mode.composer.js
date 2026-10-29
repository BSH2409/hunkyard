'use strict';
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.live = root.hunkyard.live || {};
    root.hunkyard.live.composer = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {
  // escapeHTML — delegates to the canonical window.hunkyard.shared.escapeHTML.
  var escapeHTML = (typeof window !== 'undefined' && window.hunkyard && window.hunkyard.shared)
    ? window.hunkyard.shared.escapeHTML
    : function (s) { return String(s == null ? '' : s)
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;').replace(/'/g, '&#39;'); };

  // chipLabel is the canonical version from hunkyard-comment-card-helpers.js.
  var chipLabel = (typeof window !== 'undefined' && window.hunkyard && window.hunkyard.commentCardHelpers)
    ? window.hunkyard.commentCardHelpers.chipLabel
    : function () { return 'pin'; };

  function renderComposerHTML(a) {
    var label = chipLabel(a);
    return [
      '<div class="hunkyard-live-composer" role="dialog" aria-label="New live pin">',
        '<div class="hunkyard-live-composer-meta">',
          '<span class="hunkyard-live-composer-chip">' + escapeHTML(label) + '</span>',
        '</div>',
        '<textarea class="hunkyard-live-composer-body" placeholder="Leave a live comment… (Ctrl+Enter to submit, Escape to cancel)" rows="4"></textarea>',
        '<div class="hunkyard-live-composer-error" hidden></div>',
        '<div class="hunkyard-live-composer-actions">',
          '<button type="button" class="btn btn-sm hunkyard-live-composer-cancel">Cancel</button>',
          '<button type="button" class="btn btn-sm btn-primary hunkyard-live-composer-save">Comment</button>',
        '</div>',
      '</div>',
    ].join('');
  }
  return { renderComposerHTML, escapeHTML, chipLabel };
});
