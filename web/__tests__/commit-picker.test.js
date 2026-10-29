'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { escapeHtml, relativeTime } = require('../hunkyard-comment-card-helpers.js');

const appJs = fs.readFileSync(path.join(__dirname, '..', 'app.js'), 'utf8');

function commitTime() {
  const start = appJs.indexOf('function commitTimeHTML(');
  assert.notEqual(start, -1, 'shared commit timestamp renderer must exist');
  const end = appJs.indexOf('\n  function ', start + 1);
  return new Function('escapeHtml', 'relativeTime', appJs.slice(start, end) + '\nreturn commitTimeHTML;')(escapeHtml, relativeTime);
}

test('recent commits show a relative timestamp with the full local time available', () => {
  const date = new Date(Date.now() - 3 * 3600 * 1000);
  const html = commitTime()(date.toISOString());
  assert.match(html, />3h ago<\/time>/);
  assert.ok(html.includes('title="' + escapeHtml(date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'long' })) + '"'));
});

test('commits at least 24 hours old show their full readable local timestamp', () => {
  const date = new Date(Date.now() - 24 * 3600 * 1000);
  const html = commitTime()(date.toISOString());
  assert.ok(html.includes('>' + escapeHtml(date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'long' })) + '</time>'));
  assert.doesNotMatch(html, /ago/);
});

test('missing or invalid commit dates produce no timestamp', () => {
  assert.equal(commitTime()(''), '');
  assert.equal(commitTime()('invalid'), '');
});
