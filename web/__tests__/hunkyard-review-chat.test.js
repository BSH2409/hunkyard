const { test } = require('node:test');
const assert = require('node:assert/strict');
const { transcript, isAgent, recipients, latestResponses } = require('../hunkyard-review-chat.js');

test('group transcript interleaves human messages and both agents chronologically', () => {
  const messages = transcript([
    { id: 'question', author_kind: 'human', body: 'Discuss this', created_at: '2026-01-01T00:00:00Z', replies: [
      { id: 'claude', author: 'Claude', author_kind: 'agent', created_at: '2026-01-01T00:00:03Z' },
      { id: 'codex', author: 'Codex', author_kind: 'agent', created_at: '2026-01-01T00:00:01Z' },
    ] },
    { id: 'followup', author_kind: 'human', created_at: '2026-01-01T00:00:02Z' },
  ]);
  assert.deepEqual(messages.map(entry => entry.key), ['question', 'question/codex', 'followup', 'question/claude']);
  assert.deepEqual(messages.map(entry => isAgent(entry.message)), [false, true, false, true]);
  assert.equal(isAgent({ author: 'Claude', author_kind: 'human' }), false);
});

test('messages address both invited agents or an explicitly mentioned participant', () => {
  assert.deepEqual(recipients('What do you think?', ['claude', 'codex']), ['claude', 'codex']);
  assert.deepEqual(recipients('@Claude check naming', ['claude', 'codex']), ['claude']);
});

test('timestamp precision does not move a reply before its parent', () => {
  const messages = transcript([{ id: 'parent', created_at: '2026-01-01T00:00:00Z', replies: [{ id: 'reply', created_at: '2026-01-01T00:00:00.123Z' }] }]);
  assert.deepEqual(messages.map(entry => entry.key), ['parent', 'parent/reply']);
  assert.deepEqual(recipients('@Claude review this', []), ['claude']);
});

test('a retry replaces an earlier participant failure without hiding other agents', () => {
  const entries = latestResponses([
    { id: 'first', comment_id: 'question', responses: [{ agent: 'claude', status: 'authentication' }, { agent: 'codex', status: 'replied' }] },
    { id: 'retry', comment_id: 'question', responses: [{ agent: 'claude', status: 'replied' }] },
  ]);
  assert.equal(entries.length, 2);
  assert.equal(entries.find(entry => entry.response.agent === 'claude').request.id, 'retry');
});
