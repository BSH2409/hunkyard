'use strict';
const test = require('node:test');
const assert = require('node:assert');

const overlay = require('../agent-marker-overlay.js');

test('exports expected API', () => {
  assert.equal(typeof overlay.createOverlay, 'function');
  assert.equal(typeof overlay.applyRects, 'function');
  assert.equal(typeof overlay.makeMarker, 'function');
  assert.equal(typeof overlay.setMarkersTabindex, 'function');
});

function makeFakeDoc() {
  const doc = {
    body: { appendChild: function (n) { this._appended = n; } },
    documentElement: { appendChild: function (n) { this._appended = n; } },
    createElement: (tag) => ({
      tagName: tag.toUpperCase(),
      style: {},
      _attrs: {},
      _children: [],
      _listeners: {},
      setAttribute(k, v) { this._attrs[k] = v; },
      getAttribute(k) { return this._attrs[k]; },
      appendChild(n) { this._children.push(n); },
      addEventListener(t, fn) { (this._listeners[t] = this._listeners[t] || []).push(fn); },
      get textContent() { return this._t || ''; },
      set textContent(v) { this._t = v; },
      get className() { return this._attrs.class || ''; },
      set className(v) { this._attrs.class = v; },
    }),
  };
  return doc;
}

test('createOverlay builds a root with pointer-events: none and high z-index', () => {
  const fakeDoc = makeFakeDoc();
  const ov = overlay.createOverlay(fakeDoc);
  assert.ok(ov.root);
  assert.equal(fakeDoc.documentElement._appended, ov.root);
  assert.equal(ov.root._attrs['aria-hidden'], 'true');
  assert.equal(ov.root._attrs.id, 'hunkyard-marker-root');
  assert.equal(ov.root.style.position, 'fixed');
  assert.equal(ov.root.style.top, '0');
  assert.equal(ov.root.style.left, '0');
  assert.equal(ov.root.style.pointerEvents, 'none');
  assert.equal(ov.root.style.zIndex, '2147483600');
});

test('makeMarker creates an element with role/tabindex/data-pin-id', () => {
  const fakeDoc = makeFakeDoc();
  const m = overlay.makeMarker(fakeDoc, { id: 'xyz' }, 0);
  assert.equal(m._attrs['data-pin-id'], 'xyz');
  assert.equal(m._attrs['tabindex'], '0');
  assert.equal(m._attrs.role, 'button');
  assert.equal(m.textContent, '1');
});

test('makeMarker uses pin.pin_number when present', () => {
  const fakeDoc = makeFakeDoc();
  const m = overlay.makeMarker(fakeDoc, { id: 'xyz', pin_number: 7 }, 0);
  assert.equal(m.textContent, '7');
  assert.equal(m._attrs['aria-label'], 'Comment 7');
});

test('applyRects reads all rects before writing positions', () => {
  const reads = [];
  const writes = [];
  function makeStyleProxy(label) {
    const obj = {};
    return new Proxy(obj, {
      set(t, k, v) {
        if (k === 'transform') writes.push(label + ':' + v);
        t[k] = v; return true;
      },
      get(t, k) { return t[k]; },
    });
  }
  const targets = [
    { getBoundingClientRect: () => { reads.push('a'); return { left: 1, top: 2 }; } },
    { getBoundingClientRect: () => { reads.push('b'); return { left: 3, top: 4 }; } },
  ];
  const markers = [
    { target: targets[0], el: { style: makeStyleProxy('a') } },
    { target: targets[1], el: { style: makeStyleProxy('b') } },
  ];
  overlay.applyRects(markers);
  assert.deepEqual(reads, ['a', 'b']);
  assert.deepEqual(writes, ['a:translate(1px, 2px)', 'b:translate(3px, 4px)']);
});

test('applyRects hides marker when target is null', () => {
  const m = { target: null, el: { style: {} } };
  overlay.applyRects([m]);
  assert.equal(m.el.style.display, 'none');
});

test('applyRects writes viewport coordinates for markers in a fixed overlay', () => {
  // The overlay is fixed to the viewport, so a marker goes where the element
  // is on screen, whatever scrolls (window, body or a panel) and wherever the
  // body sits; scroll offsets are not added.
  const writes = [];
  function makeStyleProxy(label) {
    const obj = {};
    return new Proxy(obj, {
      set(t, k, v) {
        if (k === 'transform') writes.push(label + ':' + v);
        t[k] = v; return true;
      },
      get(t, k) { return t[k]; },
    });
  }
  const target = { getBoundingClientRect: () => ({ left: 100, top: 400 }) };
  const win = { scrollX: 0, scrollY: 400 };
  overlay.applyRects([{ target, el: { style: makeStyleProxy('a') } }], win);
  assert.deepEqual(writes, ['a:translate(100px, 400px)']);
});

test('the overlay is fixed to the viewport and lives outside the body', () => {
  const appended = [];
  const doc = {
    documentElement: { appendChild: (n) => appended.push(['html', n]) },
    body: { appendChild: (n) => appended.push(['body', n]) },
    createElement: () => ({ style: {}, setAttribute() {} }),
  };
  const { root } = overlay.createOverlay(doc);
  assert.equal(root.style.position, 'fixed');
  assert.deepEqual(appended.map((a) => a[0]), ['html']);
});

test('applyRects without scroll offsets behaves as before (back-compat)', () => {
  const writes = [];
  function makeStyleProxy(label) {
    const obj = {};
    return new Proxy(obj, {
      set(t, k, v) {
        if (k === 'transform') writes.push(label + ':' + v);
        t[k] = v; return true;
      },
      get(t, k) { return t[k]; },
    });
  }
  const target = { getBoundingClientRect: () => ({ left: 5, top: 10 }) };
  const markers = [{ target, el: { style: makeStyleProxy('a') } }];
  overlay.applyRects(markers); // no win arg -> treat scroll as 0
  assert.deepEqual(writes, ['a:translate(5px, 10px)']);
});

test('applyRects hides marker when target is disconnected from DOM', () => {
  const target = {
    isConnected: false,
    getBoundingClientRect: () => { throw new Error('should not be called'); },
  };
  const m = { target, el: { style: {} } };
  overlay.applyRects([m]);
  assert.equal(m.el.style.display, 'none');
});

test('applyRects positions marker when target is connected with zero-size rect', () => {
  const target = {
    isConnected: true,
    getBoundingClientRect: () => ({ left: 0, top: 0, width: 0, height: 0 }),
  };
  const m = { target, el: { style: {} } };
  overlay.applyRects([m]);
  assert.equal(m.el.style.display, '');
  assert.equal(m.el.style.transform, 'translate(0px, 0px)');
});

test('applyRects shows marker when target is connected with non-zero rect', () => {
  const target = {
    isConnected: true,
    getBoundingClientRect: () => ({ left: 10, top: 20, width: 100, height: 50 }),
  };
  const m = { target, el: { style: {} } };
  overlay.applyRects([m]);
  assert.equal(m.el.style.display, '');
  assert.equal(m.el.style.transform, 'translate(10px, 20px)');
});

test('setMarkersTabindex toggles all markers atomically', () => {
  const m1 = { _attrs: { tabindex: '0' }, setAttribute(k, v) { this._attrs[k] = v; } };
  const m2 = { _attrs: { tabindex: '0' }, setAttribute(k, v) { this._attrs[k] = v; } };
  const markersById = new Map([['p1', { el: m1 }], ['p2', { el: m2 }]]);
  overlay.setMarkersTabindex(markersById, '-1');
  assert.equal(m1._attrs.tabindex, '-1');
  assert.equal(m2._attrs.tabindex, '-1');
  overlay.setMarkersTabindex(markersById, '0');
  assert.equal(m1._attrs.tabindex, '0');
  assert.equal(m2._attrs.tabindex, '0');
});

// TODO: This test previously reimplemented the keyboard handler locally instead
// of exercising the real handler from hunkyard-agent.js (which runs inside the iframe
// and wires up during initialization). It passed regardless of what the real code
// did. Replace with an E2E test in test/e2e/ that verifies Enter/Space on markers in
// live-mode triggers pin-clicked via the actual iframe agent wiring.

test('applyRects hides a marker whose element is scrolled out of view inside its panel', () => {
  const scroller = { parentElement: null, getBoundingClientRect: () => ({ left: 0, top: 0, right: 800, bottom: 600 }) };
  const win = { scrollX: 0, scrollY: 0, getComputedStyle: (el) => (el === scroller ? { overflowX: 'hidden', overflowY: 'auto' } : { overflowX: 'visible', overflowY: 'visible' }) };
  const hidden = { el: { style: {} }, target: { parentElement: scroller, getBoundingClientRect: () => ({ left: 10, top: 900, right: 90, bottom: 940 }) } };
  const shown = { el: { style: {} }, target: { parentElement: scroller, getBoundingClientRect: () => ({ left: 10, top: 100, right: 90, bottom: 140 }) } };
  overlay.applyRects([hidden, shown], win);
  assert.equal(hidden.el.style.display, 'none');
  assert.equal(shown.el.style.display, '');
  assert.equal(shown.el.style.transform, 'translate(10px, 100px)');
});

test('in Comment (pin) mode markers are dimmed and let the pointer through to the page', () => {
  const a = { el: { style: {} } };
  const b = { el: { style: {} } };
  const markers = new Map([['a', a], ['b', b]]);
  overlay.setMarkersPassThrough(markers, true);
  assert.equal(a.el.style.pointerEvents, 'none');
  assert.equal(b.el.style.opacity, '0.45');
  overlay.setMarkersPassThrough(markers, false);
  assert.equal(a.el.style.pointerEvents, 'auto', 'back to clickable for deep-links');
  assert.equal(b.el.style.opacity, '');
});

test('applyRects hides a marker whose element is covered by something on top of it', () => {
  const target = { contains: (n) => n === target || n === child, getBoundingClientRect: () => ({ left: 10, top: 10, right: 110, bottom: 50, width: 100, height: 40 }) };
  const child = {};
  const panel = { contains: () => false };
  let hit = panel;
  const win = { document: { elementFromPoint: () => hit } };
  const covered = { el: { style: {} }, target };
  overlay.applyRects([covered], win);
  assert.equal(covered.el.style.display, 'none', 'a panel on top hides the marker');
  hit = child;
  overlay.applyRects([covered], win);
  assert.equal(covered.el.style.display, '', 'the element (or its content) on top shows it');
  hit = target;
  overlay.applyRects([covered], win);
  assert.equal(covered.el.style.display, '');
});

test('a marker does not count as covering its own element', () => {
  const target = { contains: (n) => n === target, getBoundingClientRect: () => ({ left: 10, top: 10, width: 100, height: 40 }) };
  const marker = { closest: (sel) => (sel.includes('#hunkyard-marker-root') ? {} : null) };
  const win = { document: { elementsFromPoint: () => [marker, target] } };
  const m = { el: { style: {} }, target };
  overlay.applyRects([m], win);
  assert.equal(m.el.style.display, '');
});
