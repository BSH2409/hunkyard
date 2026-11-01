'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { captureTargetFor, captureBand, isBlank, capturePin } = require('../agent-capture.js');
const { MARKER_STYLE, drawMarker } = require('../agent-pin-marker.js');

function el(props) {
  return Object.assign({ parentElement: null, scrollHeight: 0, clientHeight: 0, tagName: 'DIV' }, props);
}
function fakeWin(styles) {
  return { getComputedStyle: e => ({ overflowY: styles.get(e) || 'visible' }), scrollX: 0, scrollY: 0, innerWidth: 1280, innerHeight: 800 };
}

test('captureTargetFor picks the scrolling ancestor, else the document element', () => {
  const root = el({ tagName: 'HTML', scrollHeight: 800, clientHeight: 800 });
  const doc = { documentElement: root, body: el({ tagName: 'BODY', parentElement: root }) };
  const scroller = el({ parentElement: doc.body, scrollHeight: 3000, clientHeight: 600 });
  const button = el({ parentElement: scroller });
  const win = fakeWin(new Map([[scroller, 'auto']]));
  assert.equal(captureTargetFor(button, doc, win), scroller);
  const loose = el({ parentElement: doc.body });
  assert.equal(captureTargetFor(loose, doc, win), root);
  const clipped = el({ parentElement: doc.body, scrollHeight: 3000, clientHeight: 600 });
  assert.equal(captureTargetFor(el({ parentElement: clipped }), doc, win), root, 'overflow visible is not a scroller');
});

test('captureBand keeps short pages whole and centres a 16,384 px band on the element', () => {
  assert.deepEqual(captureBand(2000, 1500, 40), { offsetY: 0, height: 2000 });
  assert.deepEqual(captureBand(40000, 30000, 40), { offsetY: 30020 - 8192, height: 16384 });
  assert.deepEqual(captureBand(40000, 100, 40), { offsetY: 0, height: 16384 });
  assert.deepEqual(captureBand(40000, 39990, 10), { offsetY: 40000 - 16384, height: 16384 });
});

test('isBlank is true only when every sampled pixel is the same', () => {
  const uniform = { data: new Uint8ClampedArray(4 * 100).fill(255), width: 10, height: 10 };
  assert.equal(isBlank(uniform), true);
  const varied = { data: new Uint8ClampedArray(4 * 100).fill(255), width: 10, height: 10 };
  varied.data[4 * 55] = 0;
  assert.equal(isBlank(varied), false);
});

// A fake canvas whose PNG size halves with each halving of the scale.
function fakeEnv({ pageHeight = 2000, elementRect = { left: 100, top: 1500, width: 120, height: 40 }, pixel = i => (i % 7) * 30, bytesAtScale1 = 1000 } = {}) {
  const root = el({ tagName: 'HTML', scrollHeight: pageHeight, clientHeight: 800, scrollWidth: 1280 });
  const doc = { documentElement: root, body: el({ tagName: 'BODY', parentElement: root }), createElement: () => makeCanvas(1, 1) };
  root.getBoundingClientRect = () => ({ left: 0, top: 0, width: 1280, height: pageHeight });
  const element = el({ parentElement: doc.body, getBoundingClientRect: () => elementRect });
  function makeCanvas(width, height) {
    const ctx = {
      drawImage() {}, strokeRect() {}, fillRect() {}, fillText() {},
      getImageData: (x, y, w, h) => ({ width: w, height: h, data: Uint8ClampedArray.from({ length: w * h * 4 }, (_, i) => pixel(i)) }),
    };
    return {
      width, height,
      getContext: () => ctx,
      toBlob: (cb) => cb({ arrayBuffer: async () => new ArrayBuffer(Math.ceil(bytesAtScale1 * (width / 1280) * (width / 1280))) }),
    };
  }
  const snapdom = async (target, options) => ({ toCanvas: async () => makeCanvas(Math.round(1280 * options.scale), Math.round(pageHeight * options.scale)) });
  return { element, deps: { snapdom, doc, win: fakeWin(new Map()), drawMarker, style: MARKER_STYLE, makeCanvas, maxBytes: 8 * 1024 * 1024 } };
}

test('capturePin returns the PNG, its size and the marker rectangle in image pixels', async () => {
  const { element, deps } = fakeEnv();
  const shot = await capturePin(element, deps);
  assert.equal(shot.ok, true, shot.reason);
  assert.equal(shot.width, 1280);
  assert.equal(shot.height, 2000);
  assert.equal(shot.scale, 1);
  assert.equal(shot.band_offset_y, 0);
  assert.deepEqual(shot.marker_rect, { x: 100, y: 1500, width: 120, height: 40 });
  assert.deepEqual(shot.viewport, { width: 1280, height: 800, scroll_x: 0, scroll_y: 0 });
  assert.ok(shot.bytes.byteLength > 0);
});

test('capturePin rejects a blank capture', async () => {
  const { element, deps } = fakeEnv({ pixel: () => 255 });
  assert.deepEqual(await capturePin(element, deps), { ok: false, reason: 'blank' });
});

test('capturePin rejects a marker that would fall outside the image', async () => {
  const { element, deps } = fakeEnv({ elementRect: { left: 100, top: 5000, width: 50, height: 20 } });
  assert.deepEqual(await capturePin(element, deps), { ok: false, reason: 'marker-outside' });
});

test('capturePin halves the scale until the PNG fits and reports it', async () => {
  const { element, deps } = fakeEnv({ bytesAtScale1: 4000 });
  deps.maxBytes = 1100;
  const shot = await capturePin(element, deps);
  assert.equal(shot.ok, true, shot.reason);
  assert.equal(shot.scale, 0.5);
  assert.deepEqual(shot.marker_rect, { x: 50, y: 750, width: 60, height: 20 });
});

test('capturePin reports a capture error instead of throwing', async () => {
  const { element, deps } = fakeEnv();
  deps.snapdom = async () => { throw new Error('canvas too large'); };
  const shot = await capturePin(element, deps);
  assert.equal(shot.ok, false);
  assert.match(shot.reason, /^error: canvas too large/);
});

function scrollerEnv(snapdomImpl) {
  const root = el({ tagName: 'HTML', scrollHeight: 800, clientHeight: 800 });
  const doc = { documentElement: root, body: el({ tagName: 'BODY', parentElement: root }), createElement: () => ({}) };
  const attrs = { style: 'color: red' };
  const style = {};
  const scroller = el({
    parentElement: doc.body, scrollHeight: 1875, clientHeight: 600, scrollWidth: 800, scrollTop: 300, scrollLeft: 0,
    style: { setProperty: (k, v) => { style[k] = v; } },
    getAttribute: (k) => (k in attrs ? attrs[k] : null),
    setAttribute: (k, v) => { attrs[k] = v; for (const key of Object.keys(style)) delete style[key]; },
    removeAttribute: (k) => { delete attrs[k]; },
    getBoundingClientRect: () => ({ left: 0, top: 0, width: 800, height: 600 }),
  });
  const element = el({ parentElement: scroller, getBoundingClientRect: () => ({ left: 12, top: 1515, width: 80, height: 45 }) });
  const win = fakeWin(new Map([[scroller, 'auto']]));
  return { scroller, attrs, style, element, deps: { snapdom: snapdomImpl, doc, win, drawMarker, style: MARKER_STYLE } };
}

test('capturePin lets an inner scroller show all its content while capturing, then restores it', async () => {
  let during = null;
  const env = scrollerEnv(async (target) => {
    during = { height: env.style.height, overflow: env.style.overflow, scrollTop: target.scrollTop };
    throw new Error('stop after the call');
  });
  const shot = await capturePin(env.element, env.deps);
  assert.equal(shot.ok, false);
  assert.deepEqual(during, { height: '1875px', overflow: 'visible', scrollTop: 0 });
  assert.equal(env.attrs.style, 'color: red', 'inline style restored after a failed capture');
  assert.equal(env.scroller.scrollTop, 300, 'scroll position restored');
});

test('two overlapping captures in one scroller leave the page as it was', async () => {
  const releases = [];
  const env = scrollerEnv(() => new Promise((resolve, reject) => { releases.push(() => reject(new Error('done'))); }));
  const first = capturePin(env.element, env.deps);
  await new Promise((r) => setImmediate(r));
  const second = capturePin(env.element, env.deps);
  await new Promise((r) => setImmediate(r));
  releases.shift()();
  await first;
  await new Promise((r) => setImmediate(r));
  if (releases.length) releases.shift()();
  await second;
  assert.equal(env.attrs.style, 'color: red', 'inline style restored');
  assert.equal(env.scroller.scrollTop, 300, 'scroll position restored');
});

test('capturePin keeps a screenshot under the 5 MiB upload limit by default', async () => {
  const { element, deps } = fakeEnv({ bytesAtScale1: 6 * 1024 * 1024 });
  delete deps.maxBytes;
  const shot = await capturePin(element, deps);
  assert.equal(shot.ok, true, shot.reason);
  assert.equal(shot.scale, 0.5, 'a 6 MiB capture is halved instead of failing to upload');
  assert.ok(shot.bytes.byteLength <= 5 * 1024 * 1024);
});

test('capturePin boxes the element on a 2x (Retina) screen', async () => {
  const { element, deps } = fakeEnv();
  const snap = deps.snapdom;
  // SnapDOM paints at devicePixelRatio unless told otherwise.
  deps.snapdom = async (target, options) => {
    const shot = await snap(target, Object.assign({}, options, { scale: (options.scale || 1) * (options.dpr === 1 ? 1 : 2) }));
    return shot;
  };
  const shot = await capturePin(element, deps);
  assert.equal(shot.ok, true, shot.reason);
  const sx = shot.width / 1280;
  assert.deepEqual(shot.marker_rect, { x: Math.round(100 * sx), y: Math.round(1500 * sx), width: Math.round(120 * sx), height: Math.round(40 * sx) },
    'the box must sit on the element in image pixels');
});

test('capturePin maps the box by the canvas density even when it is not the one asked for', async () => {
  const { element, deps } = fakeEnv();
  const snap = deps.snapdom;
  deps.snapdom = (target, options) => snap(target, Object.assign({}, options, { scale: 2 }));
  const shot = await capturePin(element, deps);
  assert.equal(shot.ok, true, shot.reason);
  assert.equal(shot.width, 2560);
  assert.deepEqual(shot.marker_rect, { x: 200, y: 3000, width: 240, height: 80 });
});

test('capturePin captures a panel that also scrolls sideways at its full width and boxes the element in place', async () => {
  const root = el({ tagName: 'HTML', scrollHeight: 800, clientHeight: 800 });
  const doc = { documentElement: root, body: el({ tagName: 'BODY', parentElement: root }) };
  const style = {};
  const attrs = {};
  const panel = el({
    parentElement: doc.body, scrollHeight: 600, clientHeight: 600, scrollWidth: 1600, clientWidth: 1100, scrollTop: 0, scrollLeft: 300,
    style: { setProperty: (k, v) => { style[k] = v; } },
    getAttribute: (k) => (k in attrs ? attrs[k] : null),
    setAttribute: (k, v) => { attrs[k] = v; },
    removeAttribute: (k) => { delete attrs[k]; for (const key of Object.keys(style)) delete style[key]; },
    // Laid out at its expanded width while capturing, scrolled sideways before.
    getBoundingClientRect: () => ({ left: 0, top: 0, width: style.width ? parseInt(style.width, 10) : 1100, height: 600 }),
  });
  // Before expansion the cell sits 300px left of where it lies in the panel's content.
  const cell = el({ parentElement: panel, getBoundingClientRect: () => (style.width ? { left: 1300, top: 100, width: 200, height: 40 } : { left: 1000, top: 100, width: 200, height: 40 }) });
  const win = fakeWin(new Map([[panel, 'auto']]));
  win.getComputedStyle = (e) => ({ overflowY: e === panel ? 'auto' : 'visible', overflowX: e === panel ? 'auto' : 'visible' });
  const makeCanvas = (w, h) => ({
    width: w, height: h,
    getContext: () => ({ drawImage() {}, strokeRect() {}, fillRect() {}, fillText() {}, getImageData: (x, y, cw, ch) => ({ data: Uint8ClampedArray.from({ length: cw * ch * 4 }, (_, i) => (i % 7) * 30) }) }),
    toBlob: (cb) => cb({ arrayBuffer: async () => new ArrayBuffer(100) }),
  });
  const snapdom = async (target) => ({ toCanvas: async () => makeCanvas(target.getBoundingClientRect().width, 600) });
  const shot = await capturePin(cell, { snapdom, doc, win, drawMarker, style: MARKER_STYLE, makeCanvas });
  assert.equal(shot.ok, true, shot.reason);
  assert.equal(shot.width, 1600, 'the whole table, not just its visible part');
  assert.deepEqual(shot.marker_rect, { x: 1300, y: 100, width: 200, height: 40 });
  assert.equal(panel.scrollLeft, 300, 'sideways scroll restored');
});
