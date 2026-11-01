// agent-capture.js — captures the page around a pinned element as a PNG with
// the fixed pin marker, inside the live/preview frame.
// Reads: window.snapdom (vendored SnapDOM), window.hunkyard.agent.pinMarker.
(function () {
  'use strict';

  var MAX_HEIGHT = 16384;
  // The attachment store accepts up to 5 MiB (session.maxAttachmentBytes).
  var MAX_BYTES = 5 * 1024 * 1024;
  var MIN_SCALE = 1 / 16;
  var BLANK_SAMPLE = 64;

  function scrolls(overflow) {
    return overflow === 'auto' || overflow === 'scroll';
  }

  // A scroller is a panel whose content overflows it, up and down or sideways.
  function isScroller(node, win) {
    if (!node) return false;
    var tall = node.scrollHeight > node.clientHeight + 1;
    var wide = node.scrollWidth > node.clientWidth + 1;
    if (!tall && !wide) return false;
    var style = win.getComputedStyle(node);
    return (tall && scrolls(style.overflowY)) || (wide && scrolls(style.overflowX));
  }

  function captureTargetFor(element, doc, win) {
    for (var node = element.parentElement; node && node !== doc.body && node !== doc.documentElement; node = node.parentElement) {
      if (isScroller(node, win)) return node;
    }
    return doc.documentElement;
  }

  function captureBand(totalHeight, rectY, rectHeight, cap) {
    var limit = cap || MAX_HEIGHT;
    if (totalHeight <= limit) return { offsetY: 0, height: totalHeight };
    var centre = rectY + rectHeight / 2;
    var offsetY = Math.round(Math.min(Math.max(0, centre - limit / 2), totalHeight - limit));
    return { offsetY: offsetY, height: limit };
  }

  function isBlank(imageData) {
    var data = imageData.data;
    for (var i = 4; i < data.length; i += 4) {
      if (data[i] !== data[0] || data[i + 1] !== data[1] || data[i + 2] !== data[2] || data[i + 3] !== data[3]) return false;
    }
    return true;
  }

  function elementRectIn(target, element, doc, win) {
    var r = element.getBoundingClientRect();
    if (target === doc.documentElement) {
      return { x: r.left + win.scrollX, y: r.top + win.scrollY, width: r.width, height: r.height };
    }
    var t = target.getBoundingClientRect();
    return { x: r.left - t.left + target.scrollLeft, y: r.top - t.top + target.scrollTop, width: r.width, height: r.height };
  }

  // SnapDOM paints an element at its on-screen size, so an inner scroller is
  // grown to its full content for the capture and put back right after.
  // Captures run one at a time: a second one starting while the first has
  // a scroller expanded would record the expanded state as the original.
  var captureChain = Promise.resolve();
  function oneAtATime(fn) {
    var run = captureChain.then(fn, fn);
    captureChain = run.then(function () {}, function () {});
    return run;
  }

  async function withScrollerExpanded(target, doc, capture) {
    if (target === doc.documentElement) return capture();
    var style = target.getAttribute('style');
    var scrollTop = target.scrollTop;
    var scrollLeft = target.scrollLeft;
    var height = target.scrollHeight;
    var width = target.scrollWidth;
    target.style.setProperty('height', height + 'px', 'important');
    target.style.setProperty('max-height', 'none', 'important');
    if (width > target.clientWidth) {
      target.style.setProperty('width', width + 'px', 'important');
      target.style.setProperty('max-width', 'none', 'important');
    }
    target.style.setProperty('overflow', 'visible', 'important');
    target.scrollTop = 0;
    target.scrollLeft = 0;
    try {
      return await capture();
    } finally {
      if (style === null) target.removeAttribute('style');
      else target.setAttribute('style', style);
      target.scrollTop = scrollTop;
      target.scrollLeft = scrollLeft;
    }
  }

  function blobBytes(canvas) {
    return new Promise(function (resolve, reject) {
      canvas.toBlob(function (blob) {
        if (!blob) { reject(new Error('png encoding failed')); return; }
        blob.arrayBuffer().then(resolve, reject);
      }, 'image/png');
    });
  }

  async function capturePin(element, deps) {
    var doc = deps.doc;
    var win = deps.win;
    var makeCanvas = deps.makeCanvas || function (width, height) {
      var c = doc.createElement('canvas');
      c.width = width;
      c.height = height;
      return c;
    };
    var maxBytes = deps.maxBytes || MAX_BYTES;
    try {
      var options = { scale: 1, dpr: 1, embedFonts: true };
      if (deps.exclude) options.exclude = deps.exclude;
      var cssRect;
      var cssWidth;
      var full = await oneAtATime(function () {
        var target = captureTargetFor(element, doc, win);
        return withScrollerExpanded(target, doc, async function () {
          // Measured with the panel expanded: widening it can reflow its content.
          cssRect = elementRectIn(target, element, doc, win);
          cssWidth = target === doc.documentElement ? target.scrollWidth : target.getBoundingClientRect().width;
          return (await deps.snapdom(target, options)).toCanvas();
        });
      });
      // The element is measured in CSS pixels; the canvas may be painted at
      // another density (a 2x screen), so map the box into canvas pixels.
      var ratio = cssWidth > 0 ? full.width / cssWidth : 1;
      var rect = { x: cssRect.x * ratio, y: cssRect.y * ratio, width: cssRect.width * ratio, height: cssRect.height * ratio };
      var band = captureBand(full.height, rect.y, rect.height);
      var inBand = { x: rect.x, y: rect.y - band.offsetY, width: rect.width, height: rect.height };
      if (inBand.x < 0 || inBand.y < 0 || inBand.x + inBand.width > full.width || inBand.y + inBand.height > band.height) {
        return { ok: false, reason: 'marker-outside' };
      }
      var sample = makeCanvas(BLANK_SAMPLE, BLANK_SAMPLE);
      var sampleCtx = sample.getContext('2d');
      sampleCtx.drawImage(full, 0, band.offsetY, full.width, band.height, 0, 0, BLANK_SAMPLE, BLANK_SAMPLE);
      if (isBlank(sampleCtx.getImageData(0, 0, BLANK_SAMPLE, BLANK_SAMPLE))) return { ok: false, reason: 'blank' };
      for (var scale = 1; scale >= MIN_SCALE; scale /= 2) {
        var width = Math.round(full.width * scale);
        var height = Math.round(band.height * scale);
        var out = makeCanvas(width, height);
        var ctx = out.getContext('2d');
        ctx.drawImage(full, 0, band.offsetY, full.width, band.height, 0, 0, width, height);
        var markerRect = {
          x: Math.round(inBand.x * scale),
          y: Math.round(inBand.y * scale),
          width: Math.round(inBand.width * scale),
          height: Math.round(inBand.height * scale),
        };
        deps.drawMarker(ctx, markerRect, { width: width, height: height }, deps.style);
        var bytes = await blobBytes(out);
        if (bytes.byteLength <= maxBytes) {
          return {
            ok: true,
            bytes: bytes,
            width: width,
            height: height,
            scale: scale,
            band_offset_y: band.offsetY,
            marker_rect: markerRect,
            viewport: { width: win.innerWidth, height: win.innerHeight, scroll_x: win.scrollX, scroll_y: win.scrollY },
          };
        }
      }
      return { ok: false, reason: 'too-large' };
    } catch (err) {
      return { ok: false, reason: 'error: ' + (err && err.message ? err.message : String(err)) };
    }
  }

  var api = { captureTargetFor: captureTargetFor, captureBand: captureBand, isBlank: isBlank, capturePin: capturePin };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.agent = window.hunkyard.agent || {};
    window.hunkyard.agent.capture = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
