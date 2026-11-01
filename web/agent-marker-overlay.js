'use strict';
//
// Marker overlay — coordinate-system reasoning (load-bearing; do not delete).
//
//   * The overlay root is `position: fixed` at the viewport origin and is
//     attached to <html>, outside <body>: a positioned, offset or transformed
//     body would otherwise move the origin markers are measured from.
//
//   * Markers are placed at the element's viewport rect
//     (`getBoundingClientRect`), with no scroll offsets. That holds whatever
//     scrolls — the window, the body or a panel inside the page.
//
//   * Viewport rects change on any scroll, so the agent re-measures on scroll
//     (capture phase, which also sees inner panels), resize and DOM changes,
//     once per frame. A marker whose element is clipped by a scrolling panel
//     or not found is hidden.
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.agent = root.hunkyard.agent || {};
    root.hunkyard.agent.markers = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {

  function createOverlay(doc) {
    const root = doc.createElement('div');
    root.setAttribute('id', 'hunkyard-marker-root');
    root.setAttribute('aria-hidden', 'true');
    // Fixed at the viewport origin, outside <body>. See top-of-file comment.
    root.style.position = 'fixed';
    root.style.top = '0';
    root.style.left = '0';
    root.style.width = '0';
    root.style.height = '0';
    root.style.pointerEvents = 'none';
    root.style.zIndex = '2147483600';
    doc.documentElement.appendChild(root);
    // markersById: pin_id -> { el, anchor, status, element, rect }
    const markersById = new Map();
    return { root, markersById };
  }

  function makeMarker(doc, pin, index) {
    const el = doc.createElement('div');
    el.className = 'hunkyard-live-marker';
    el.setAttribute('role', 'button');
    el.setAttribute('tabindex', '0');
    el.setAttribute('data-pin-id', pin.id);
    // Pin number is GLOBAL within the review (REVISION). Fall back to index+1
    // for tests/back-compat, but production set-pins payloads carry pin_number.
    const number = (typeof pin.pin_number === 'number') ? pin.pin_number : (index + 1);
    el.setAttribute('aria-label', 'Comment ' + number);
    el.style.position = 'absolute';
    el.style.top = '0';
    el.style.left = '0';
    el.style.pointerEvents = 'auto';
    el.textContent = String(number);
    return el;
  }

  // Read all rects, then write all positions (no interleave).
  // `win` is optional and defaults to the global window — pass it explicitly
  // from tests. Scroll offsets convert viewport coords (rect.left/top) into
  // document coords, so absolute-positioned markers stay anchored to the
  // element they annotate as the page scrolls.
  // clippedOut reports whether rect lies outside the visible area of an
  // ancestor that clips its content (a panel that scrolls on its own).
  function clippedOut(target, rect, win) {
    if (!win || typeof win.getComputedStyle !== 'function') return false;
    for (let node = target.parentElement; node; node = node.parentElement) {
      const style = win.getComputedStyle(node);
      const clipsX = style.overflowX !== 'visible';
      const clipsY = style.overflowY !== 'visible';
      if (!clipsX && !clipsY) continue;
      const box = node.getBoundingClientRect();
      if (clipsY && (rect.bottom <= box.top || rect.top >= box.bottom)) return true;
      if (clipsX && (rect.right <= box.left || rect.left >= box.right)) return true;
    }
    return false;
  }

  // topPageElement is the topmost element at (x, y) that is not part of
  // Hunkyard's own overlays: in Browse mode markers are hit-testable.
  function topPageElement(doc, x, y) {
    const hits = typeof doc.elementsFromPoint === 'function' ? doc.elementsFromPoint(x, y) : [doc.elementFromPoint(x, y)];
    for (const hit of hits) {
      if (!hit) continue;
      if (typeof hit.closest === 'function' && hit.closest('#hunkyard-marker-root, #hunkyard-agent-overlay')) continue;
      return hit;
    }
    return null;
  }

  // covered reports whether something else is on top of the element where
  // its marker sits and at its centre (a panel, modal or dropdown), so the
  // marker does not float over it.
  function covered(target, rect, win) {
    const doc = win && win.document;
    if (!doc || (typeof doc.elementFromPoint !== 'function' && typeof doc.elementsFromPoint !== 'function')) return false;
    const width = rect.width != null ? rect.width : (rect.right - rect.left) || 0;
    const height = rect.height != null ? rect.height : (rect.bottom - rect.top) || 0;
    const points = [
      [rect.left + Math.min(6, width / 2), rect.top + Math.min(6, height / 2)],
      [rect.left + width / 2, rect.top + height / 2],
    ];
    let sampled = false;
    for (const [x, y] of points) {
      const hit = topPageElement(doc, x, y);
      if (!hit) continue;
      sampled = true;
      if (hit === target || (typeof target.contains === 'function' && target.contains(hit)) ||
          (typeof hit.contains === 'function' && hit.contains(target))) return false;
    }
    return sampled;
  }

  function applyRects(markers, win) {
    const reads = markers.map(m => {
      if (!m.target) return null;
      if (typeof m.target.isConnected !== 'undefined' && !m.target.isConnected) return null;
      const rect = m.target.getBoundingClientRect();
      return clippedOut(m.target, rect, win) || covered(m.target, rect, win) ? null : rect;
    });
    for (let i = 0; i < markers.length; i++) {
      const m = markers[i];
      const r = reads[i];
      if (!r) { m.el.style.display = 'none'; continue; }
      m.el.style.display = '';
      m.el.style.transform = `translate(${Math.round(r.left)}px, ${Math.round(r.top)}px)`;
    }
  }

  function setMarkersTabindex(markersById, value) {
    if (!markersById || !markersById.forEach) return;
    markersById.forEach(m => {
      if (m && m.el && typeof m.el.setAttribute === 'function') {
        m.el.setAttribute('tabindex', value);
      }
    });
  }

  // While the reviewer is picking an element, markers are dimmed and let the
  // pointer through so the element underneath can be picked. Set inline so it
  // holds before the marker stylesheet has loaded.
  function setMarkersPassThrough(markersById, on) {
    if (!markersById || !markersById.forEach) return;
    markersById.forEach(m => {
      if (!m || !m.el || !m.el.style) return;
      m.el.style.pointerEvents = on ? 'none' : 'auto';
      m.el.style.opacity = on ? '0.45' : '';
    });
  }

  return { createOverlay, makeMarker, applyRects, setMarkersTabindex, setMarkersPassThrough };
});
