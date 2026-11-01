'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { MARKER_STYLE, markerGeometry, drawMarker } = require('../agent-pin-marker.js');

const image = { width: 1000, height: 800 };
// Half of the drawn line: the red stroke's half plus the white halo.
const half = MARKER_STYLE.stroke / 2 + MARKER_STYLE.halo_width;

test('the marker style is the documented constant', () => {
  assert.deepEqual(MARKER_STYLE, { color: '#FF0000', stroke: 4, halo: '#FFFFFF', halo_width: 2, padding: 6, label: '1' });
});

test('the outline sits outside the element by padding plus half the stroke', () => {
  const g = markerGeometry({ x: 400, y: 300, width: 100, height: 40 }, image, MARKER_STYLE);
  const grow = MARKER_STYLE.padding + MARKER_STYLE.stroke / 2;
  assert.deepEqual(g.outline, { x: 400 - grow, y: 300 - grow, width: 100 + 2 * grow, height: 40 + 2 * grow });
  assert.equal(g.inside, true);
});

test('the outline is clamped so the whole line stays inside the image', () => {
  const g = markerGeometry({ x: 0, y: 0, width: 1000, height: 800 }, image, MARKER_STYLE);
  assert.equal(g.outline.x, half);
  assert.equal(g.outline.y, half);
  assert.equal(g.outline.x + g.outline.width, image.width - half);
  assert.equal(g.outline.y + g.outline.height, image.height - half);
});

test('the tag sits above-left outside the outline, or inside it when there is no room', () => {
  const room = markerGeometry({ x: 400, y: 300, width: 100, height: 40 }, image, MARKER_STYLE);
  assert.ok(room.tag.y + room.tag.height <= room.outline.y - half, 'tag above the outline');
  assert.ok(room.tag.x <= room.outline.x, 'tag at the left edge');
  const top = markerGeometry({ x: 400, y: 2, width: 100, height: 40 }, image, MARKER_STYLE);
  assert.ok(top.tag.y >= top.outline.y, 'tag moved inside the outline at the top edge');
  assert.ok(top.tag.y >= 0 && top.tag.x >= 0);
});

test('an element outside the image is reported', () => {
  assert.equal(markerGeometry({ x: 1200, y: 100, width: 50, height: 20 }, image, MARKER_STYLE).inside, false);
  assert.equal(markerGeometry({ x: 100, y: -90, width: 50, height: 20 }, image, MARKER_STYLE).inside, false);
});

test('drawMarker paints the white halo, then the red line, then the red tag and white label', () => {
  const calls = [];
  const ctx = new Proxy({}, {
    set(target, key, value) { calls.push(['set', key, value]); target[key] = value; return true; },
    get(target, key) {
      if (key in target) return target[key];
      return (...args) => calls.push([key, ...args]);
    },
  });
  drawMarker(ctx, { x: 400, y: 300, width: 100, height: 40 }, image, MARKER_STYLE);
  const strokes = calls.filter(c => c[0] === 'strokeRect');
  assert.equal(strokes.length, 2);
  const styleAt = (i, key) => {
    let value;
    for (const c of calls.slice(0, i)) if (c[0] === 'set' && c[1] === key) value = c[2];
    return value;
  };
  const haloAt = calls.indexOf(strokes[0]);
  const redAt = calls.indexOf(strokes[1]);
  assert.equal(styleAt(haloAt, 'strokeStyle'), '#FFFFFF');
  assert.equal(styleAt(haloAt, 'lineWidth'), MARKER_STYLE.stroke + 2 * MARKER_STYLE.halo_width);
  assert.equal(styleAt(redAt, 'strokeStyle'), '#FF0000');
  assert.equal(styleAt(redAt, 'lineWidth'), 4);
  const fillRect = calls.findIndex(c => c[0] === 'fillRect');
  const fillText = calls.findIndex(c => c[0] === 'fillText');
  assert.ok(fillRect > redAt && fillText > fillRect);
  assert.equal(styleAt(fillRect, 'fillStyle'), '#FF0000');
  assert.equal(styleAt(fillText, 'fillStyle'), '#FFFFFF');
  assert.equal(calls[fillText][1], '1');
});
