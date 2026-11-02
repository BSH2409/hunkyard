'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const annotations = require('../hunkyard-annotations.js');

test('coverageLine summarises uncovered changed lines', () => {
  assert.equal(annotations.coverageLine({ changed: 9, uncovered: 2 }), '2 of 9 changed lines uncovered');
  assert.equal(annotations.coverageLine({ changed: 1, uncovered: 1 }), '1 of 1 changed line uncovered');
  assert.equal(annotations.coverageLine({ changed: 4, uncovered: 0 }), 'All 4 changed lines covered');
  assert.equal(annotations.coverageLine({ changed: 0, uncovered: 0 }), '');
  assert.equal(annotations.coverageLine(null), '');
});

test('uncoveredRanges unions every tool of the current round', () => {
  const tools = {
    go: { round: 2, marks: { 'a.go': [{ start: 5, end: 6, kind: 'uncovered' }, { start: 1, end: 1, kind: 'uncovered' }] } },
    js: { round: 2, marks: { 'a.go': [{ start: 3, end: 3, kind: 'uncovered' }] } },
    stale: { round: 1, marks: { 'a.go': [{ start: 9, end: 9, kind: 'uncovered' }] } },
  };
  assert.deepEqual(annotations.uncoveredRanges(tools, 'a.go', 2), [{ start: 1, end: 1 }, { start: 3, end: 3 }, { start: 5, end: 6 }]);
  assert.deepEqual(annotations.uncoveredRanges(tools, 'b.go', 2), []);
  assert.deepEqual(annotations.uncoveredRanges(null, 'a.go', 2), []);
});

test('fileCoverage merges tools of the current round', () => {
  const tools = {
    go: { round: 2, coverage: { 'a.go': { changed: 4, uncovered: 1 } } },
    stale: { round: 1, coverage: { 'a.go': { changed: 4, uncovered: 4 } } },
  };
  assert.deepEqual(annotations.fileCoverage(tools, 'a.go', 2), { changed: 4, uncovered: 1 });
  assert.equal(annotations.fileCoverage(tools, 'b.go', 2), null);
});

