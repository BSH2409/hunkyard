'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const risk = require('../hunkyard-risk.js');

const base = { path: 'lib/util.go', additions: 0, deletions: 0 };
const ctx = (extra) => Object.assign({ patterns: risk.defaultPatterns, changedPaths: [] }, extra || {});

test('each risk term counts on its own', () => {
  assert.deepEqual(risk.score({ ...base, path: 'docs/readme.md' }, ctx({ changedPaths: ['docs/readme.md'] })), { value: 2, reasons: ['no test changed for it'] });
  assert.equal(risk.score({ ...base, path: 'internal/auth/login.go' }, ctx({ changedPaths: ['internal/auth/login_test.go'] })).value, 3);
  assert.deepEqual(risk.score({ ...base, path: 'internal/auth/login.go' }, ctx({ changedPaths: ['internal/auth/login_test.go'] })).reasons, ['sensitive path']);
  assert.equal(risk.score({ ...base, additions: 7, deletions: 0 }, ctx({ changedPaths: ['lib/util_test.go'] })).value, 3);
  assert.equal(risk.score(base, ctx({ changedPaths: ['lib/util_test.go'], coverage: { uncovered: 2 } })).value, 2);
  assert.equal(risk.score(base, ctx({ changedPaths: ['lib/util_test.go'], ruleFindings: 5 })).value, 3);
});

test('a test file is never flagged for missing tests', () => {
  assert.equal(risk.score({ ...base, path: 'lib/util_test.go' }, ctx()).value, 0);
});

test('custom patterns replace nothing but add to the defaults', () => {
  assert.equal(risk.score({ ...base, path: 'billing/charge.go' }, ctx({ patterns: risk.defaultPatterns.concat(['billing/**']), changedPaths: ['billing/charge_test.go'] })).value, 3);
});

test('compare orders by score, then path', () => {
  const scores = new Map([['a.go', { value: 1 }], ['b.go', { value: 5 }], ['c.go', { value: 1 }]]);
  const sorted = ['c.go', 'a.go', 'b.go'].map((p) => ({ path: p })).sort(risk.compare(scores, (x, y) => (x < y ? -1 : x > y ? 1 : 0)));
  assert.deepEqual(sorted.map((f) => f.path), ['b.go', 'a.go', 'c.go']);
});
