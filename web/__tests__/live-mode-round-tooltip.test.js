'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { composeRoundTooltip } = require('../live-mode-round-tooltip.js');

const pins = (specs) => specs.map((s, i) => ({
  id: 'p' + i,
  resolved: !!s.resolved,
  pin_status: s.status,
  pin_checked_round: s.checked || 0,
}));

test('composes round / total / resolved / outdated / unchecked / carried counts', () => {
  const got = composeRoundTooltip({
    round: 3,
    pins: pins([
      { resolved: true, status: 'outdated', checked: 1 },
      { status: 'outdated', checked: 3 },
      { status: 'current', checked: 2 },
      { status: 'current', checked: 3 },
      {},
    ]),
  });
  assert.deepEqual(got, {
    round: 3, total: 5, resolved: 1, outdated: 1, unchecked: 2, carried: 4,
  });
});

test('zero pins yields zeros', () => {
  assert.deepEqual(
    composeRoundTooltip({ round: 1, pins: [] }),
    { round: 1, total: 0, resolved: 0, outdated: 0, unchecked: 0, carried: 0 },
  );
});
