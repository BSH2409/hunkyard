'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { start } = require('../hunkyard-activity.js');

function fakeTarget() {
  const listeners = {};
  return {
    listeners,
    addEventListener(type, fn) { (listeners[type] = listeners[type] || []).push(fn); },
    removeEventListener(type, fn) { listeners[type] = (listeners[type] || []).filter(f => f !== fn); },
    fire(type) { (listeners[type] || []).forEach(fn => fn({ type })); },
  };
}

test('no interaction sends nothing', () => {
  let posts = 0;
  const target = fakeTarget();
  start({ target, post: () => { posts++; }, throttleMs: 60000, now: () => 0 });
  assert.equal(posts, 0);
});

test('a burst of interaction inside the throttle sends one post', () => {
  let posts = 0;
  let clock = 1000;
  const target = fakeTarget();
  start({ target, post: () => { posts++; }, throttleMs: 60000, now: () => clock });
  for (const type of ['pointerdown', 'keydown', 'wheel', 'scroll', 'focusin']) target.fire(type);
  assert.equal(posts, 1);
  clock += 59000;
  target.fire('keydown');
  assert.equal(posts, 1);
  clock += 2000;
  target.fire('pointerdown');
  assert.equal(posts, 2);
});

test('stop removes the listeners', () => {
  let posts = 0;
  const target = fakeTarget();
  const tracker = start({ target, post: () => { posts++; }, throttleMs: 0, now: () => 0 });
  tracker.stop();
  target.fire('keydown');
  assert.equal(posts, 0);
});

test('mark() reports activity from elsewhere (the live iframe) under the same throttle', () => {
  let posts = 0;
  let t = 0;
  const target = fakeTarget();
  const tracker = start({ post: () => { posts++; }, throttleMs: 1000, target, now: () => t });
  tracker.mark();
  tracker.mark();
  assert.equal(posts, 1);
  t = 1500;
  target.listeners.keydown[0]();
  assert.equal(posts, 2);
  tracker.mark();
  assert.equal(posts, 2);
});
