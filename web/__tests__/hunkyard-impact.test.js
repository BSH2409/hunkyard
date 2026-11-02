'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const impact = require('../hunkyard-impact.js');

const result = {
  symbols: ['ServeLogin', 'Session'],
  callers: [{ path: 'routes.go', line: 4, symbol: 'ServeLogin', text: 'r.Get("/login", <ServeLogin>)' }, { path: 'cmd/main.go', line: 9, symbol: 'Session', text: 'Session{}' }],
  tests: [{ path: 'api_test.go', hits: 2, changed: false }, { path: 'web/login.spec.ts', hits: 1, changed: true }],
  truncated: { callers: false, tests: false },
};

test('summaryLine counts callers and tests', () => {
  assert.equal(impact.summaryLine(result), '2 callers · 2 tests · 1 test changed');
  assert.equal(impact.summaryLine({ symbols: ['A'], callers: [result.callers[0]], tests: [result.tests[0]], truncated: {} }), '1 caller · 1 test · 0 tests changed');
  assert.equal(impact.summaryLine({ symbols: ['A'], callers: [], tests: [], truncated: {} }), 'no callers · no tests');
  assert.equal(impact.summaryLine({ symbols: [], callers: [], tests: [], truncated: {} }), '');
  assert.equal(impact.summaryLine({ symbols: ['A'], callers: new Array(50).fill(result.callers[0]), tests: [], truncated: { callers: true } }), '50+ callers · no tests');
  assert.equal(impact.summaryLine(null), '');
});

test('panelHTML lists callers with links and tests with a changed badge', () => {
  const html = impact.panelHTML(result);
  assert.match(html, /data-impact-path="routes\.go" data-impact-line="4"/);
  assert.match(html, /&lt;ServeLogin&gt;/);
  assert.match(html, /api_test\.go/);
  assert.equal((html.match(/impact-changed/g) || []).length, 1);
  assert.match(html, /ServeLogin, Session/);
  assert.match(html, /text match/i);
});

test('panelHTML explains an empty result', () => {
  assert.match(impact.panelHTML({ symbols: [], callers: [], tests: [], truncated: {} }), /No changed definitions/);
  assert.match(impact.panelHTML({ symbols: ['A'], callers: [], tests: [], truncated: {} }), /No other file mentions/);
});

test('pillText is short enough for a file header', () => {
  assert.equal(impact.pillText(result), '2 callers · 2 tests');
  assert.equal(impact.pillText({ symbols: ['A'], callers: [], tests: [], truncated: {} }), 'no callers · no tests');
  assert.equal(impact.pillText({ symbols: [], callers: [], tests: [], truncated: {} }), '');
});
