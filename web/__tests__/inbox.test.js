'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const inbox = require('../inbox.js');

const entries = [
  { key: 'aaaaaaaaaaaa', label: 'feat/<login>', repository: 'orion', state: 'waiting_for_reviewer', round: 2, unresolved: 3, url: 'http://localhost:5000' },
  { key: 'bbbbbbbbbbbb', label: 'main', repository: 'orion', state: 'agent_working', round: 1, unresolved: 0, url: 'http://localhost:5002' },
  { key: 'cccccccccccc', label: 'fix/x', repository: 'blogvault', state: 'paused', round: 4, unresolved: 1 },
  { key: 'dddddddddddd', label: 'docs', repository: 'site', state: 'unreachable', round: 0, unresolved: 0, url: 'http://localhost:5004' },
];

test('rows show state, label, round and the right action', () => {
  const html = inbox.listHTML(entries);
  assert.equal((html.match(/class="inbox-row/g) || []).length, 4);
  assert.match(html, /feat\/&lt;login&gt;/);
  assert.match(html, /Waiting for you/);
  assert.match(html, /Agent working/);
  assert.match(html, /href="http:\/\/localhost:5000"/);
  assert.match(html, /data-inbox-resume="cccccccccccc"/);
  assert.equal((html.match(/data-inbox-resume=/g) || []).length, 1);
  assert.match(html, /round 4 · 1 open/);
});

test('an empty inbox says so', () => {
  assert.match(inbox.listHTML([]), /No reviews are running or paused/);
});

test('summary counts what waits for the reviewer', () => {
  assert.equal(inbox.summary(entries), '1 waiting for you · 1 agent working · 1 paused');
  assert.equal(inbox.summary([]), '');
});
