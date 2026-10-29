'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { create } = require('../hunkyard-tab-ready.js');

function fakeDocument(visibility) {
  return {
    title: 'Hunkyard Review',
    visibilityState: visibility || 'visible',
  };
}

test('notifyRoundReady badges when tab is hidden', () => {
  const doc = fakeDocument('hidden');
  const ready = create({ document: doc, baseTitle: 'Hunkyard Review' });
  const result = ready.notifyRoundReady();
  assert.equal(result.badged, true);
  assert.equal(ready.isBadgeActive(), true);
  assert.ok(doc.title.startsWith(ready.BADGE_PREFIX));
});

test('notifyRoundReady is a no-op when tab is visible', () => {
  const doc = fakeDocument('visible');
  const ready = create({ document: doc, baseTitle: 'Hunkyard Review' });
  const result = ready.notifyRoundReady();
  assert.equal(result.badged, false);
  assert.equal(ready.isBadgeActive(), false);
  assert.equal(doc.title, 'Hunkyard Review');
});

test('notifyRoundReady with force badges even when the tab is visible', () => {
  const doc = fakeDocument('visible');
  const ready = create({ document: doc, baseTitle: 'Hunkyard Review' });
  const result = ready.notifyRoundReady({ force: true });
  assert.equal(result.badged, true);
  assert.equal(ready.isBadgeActive(), true);
});

// The browser Notification path was removed on purpose: hunkyard serves on an
// ephemeral localhost port, so a permission grant never survives to the next
// run. Desktop notifications are server-side (notify_on_round_ready).
test('no browser Notification is constructed, even when one is granted globally', () => {
  const constructed = [];
  function FakeNotification(title, opts) { constructed.push({ title, opts }); }
  FakeNotification.permission = 'granted';
  const prior = globalThis.Notification;
  globalThis.Notification = FakeNotification;
  try {
    const ready = create({ document: fakeDocument('hidden'), baseTitle: 'Hunkyard Review' });
    ready.notifyRoundReady();
    assert.equal(constructed.length, 0);
    assert.equal(ready.requestPermission, undefined);
    assert.equal(ready.lastNotification, undefined);
  } finally {
    if (prior === undefined) delete globalThis.Notification;
    else globalThis.Notification = prior;
  }
});

test('clearTabBadge restores title', () => {
  const doc = fakeDocument('hidden');
  const ready = create({ document: doc, baseTitle: 'Hunkyard Review' });
  ready.notifyRoundReady();
  ready.clearTabBadge();
  assert.equal(ready.isBadgeActive(), false);
  assert.equal(doc.title, 'Hunkyard Review');
});

test('setDocumentTitle keeps the badge prefix while the badge is active', () => {
  const doc = fakeDocument('hidden');
  const ready = create({ document: doc, baseTitle: 'Hunkyard Review' });
  ready.notifyRoundReady();
  ready.setDocumentTitle('Hunkyard Review — app.js');
  assert.equal(doc.title, ready.BADGE_PREFIX + 'Hunkyard Review — app.js');
  ready.clearTabBadge();
  assert.equal(doc.title, 'Hunkyard Review — app.js');
});
