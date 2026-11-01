'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const r = require('../live-mode-round-resolve.js');

test('pinsToResolveAtRoundStart returns only pins on currentPath', () => {
  const pins = [
    { id: 'a', dom_anchor: { pathname: '/' } },
    { id: 'b', dom_anchor: { pathname: '/x' } },
    { id: 'c', dom_anchor: { pathname: '/' } },
  ];
  assert.deepEqual(r.pinsToResolveAtRoundStart(pins, '/'), ['a', 'c']);
});

test('pinsToResolveAtRoundStart skips already-resolved this round', () => {
  const pins = [
    { id: 'a', dom_anchor: { pathname: '/' }, _roundResolved: true },
    { id: 'b', dom_anchor: { pathname: '/' } },
  ];
  assert.deepEqual(r.pinsToResolveAtRoundStart(pins, '/'), ['b']);
});

test('pinStatusToReport: a status the server has not seen this round is reported', () => {
  assert.equal(r.pinStatusToReport({ pin_status: 'current', pin_checked_round: 2 }, { pin_status: 'current' }, 3), 'current');
  assert.equal(r.pinStatusToReport({ pin_status: 'current', pin_checked_round: 3 }, { pin_status: 'outdated' }, 3), 'outdated');
  assert.equal(r.pinStatusToReport({}, { pin_status: 'outdated' }, 1), 'outdated');
});

test('pinStatusToReport: nothing to report when unchanged this round or no status', () => {
  assert.equal(r.pinStatusToReport({ pin_status: 'outdated', pin_checked_round: 3 }, { pin_status: 'outdated' }, 3), null);
  assert.equal(r.pinStatusToReport({ pin_status: 'current', pin_checked_round: 1 }, { status: 'resolved' }, 3), null);
  assert.equal(r.pinStatusToReport(null, { pin_status: 'current' }, 3), null);
});
