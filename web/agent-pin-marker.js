// agent-pin-marker.js — draws the fixed pin marker onto a captured page image.
// Depends on no other window.hunkyard namespace.
(function () {
  'use strict';

  // Fixed so agents can rely on it: pure red outline with a white halo,
  // drawn outside the element so its pixels stay visible.
  var MARKER_STYLE = Object.freeze({ color: '#FF0000', stroke: 4, halo: '#FFFFFF', halo_width: 2, padding: 6, label: '1' });
  var TAG_SIZE = 22;

  function intersects(rect, image) {
    return rect.x < image.width && rect.y < image.height && rect.x + rect.width > 0 && rect.y + rect.height > 0;
  }

  function markerGeometry(rect, image, style) {
    var grow = style.padding + style.stroke / 2;
    var half = style.stroke / 2 + style.halo_width;
    var left = Math.max(half, rect.x - grow);
    var top = Math.max(half, rect.y - grow);
    var right = Math.min(image.width - half, rect.x + rect.width + grow);
    var bottom = Math.min(image.height - half, rect.y + rect.height + grow);
    var outline = { x: left, y: top, width: Math.max(0, right - left), height: Math.max(0, bottom - top) };
    var tag = { x: outline.x - half, y: outline.y - half - TAG_SIZE, width: TAG_SIZE, height: TAG_SIZE };
    if (tag.y < 0 || tag.x < 0) {
      tag = { x: outline.x + half, y: outline.y + half, width: TAG_SIZE, height: TAG_SIZE };
    }
    return { outline: outline, tag: tag, inside: intersects(rect, image) };
  }

  function drawMarker(ctx, rect, image, style) {
    var g = markerGeometry(rect, image, style);
    var o = g.outline;
    ctx.lineJoin = 'miter';
    ctx.strokeStyle = style.halo;
    ctx.lineWidth = style.stroke + 2 * style.halo_width;
    ctx.strokeRect(o.x, o.y, o.width, o.height);
    ctx.strokeStyle = style.color;
    ctx.lineWidth = style.stroke;
    ctx.strokeRect(o.x, o.y, o.width, o.height);
    ctx.fillStyle = style.color;
    ctx.fillRect(g.tag.x, g.tag.y, g.tag.width, g.tag.height);
    ctx.fillStyle = style.halo;
    ctx.font = 'bold 15px sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(style.label, g.tag.x + g.tag.width / 2, g.tag.y + g.tag.height / 2 + 1);
    return g;
  }

  var api = { MARKER_STYLE: MARKER_STYLE, markerGeometry: markerGeometry, drawMarker: drawMarker };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.agent = window.hunkyard.agent || {};
    window.hunkyard.agent.pinMarker = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
