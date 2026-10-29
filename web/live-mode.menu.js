'use strict';
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.live = root.hunkyard.live || {};
    root.hunkyard.live.menu = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {
  // escapeHTML — delegates to the canonical window.hunkyard.shared.escapeHTML.
  var escapeHTML = (typeof window !== 'undefined' && window.hunkyard && window.hunkyard.shared)
    ? window.hunkyard.shared.escapeHTML
    : function (s) { return String(s == null ? '' : s)
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;').replace(/'/g, '&#39;'); };
  function renderAncestorMenuHTML(options) {
    var items = options.map(function (o) {
      return '<button type="button" class="hunkyard-live-ancestor-menu-item" data-level="' + o.level + '">' + escapeHTML(o.label) + '</button>';
    }).join('');
    return '<div class="hunkyard-live-ancestor-menu" role="menu">' + items + '</div>';
  }
  function clampMenuPosition(opts) {
    var x = opts.x, y = opts.y;
    var width = opts.width, height = opts.height;
    var vw = opts.vw, vh = opts.vh;
    var pad = (opts.pad == null) ? 8 : opts.pad;
    var maxX = Math.max(pad, vw - width - pad);
    var maxY = Math.max(pad, vh - height - pad);
    return {
      x: Math.min(Math.max(pad, x), maxX),
      y: Math.min(Math.max(pad, y), maxY),
    };
  }
  return { renderAncestorMenuHTML, clampMenuPosition };
});
