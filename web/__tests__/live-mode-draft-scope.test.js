'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

// live-mode.js is one large IIFE, so this checks its source: drafts must be
// scoped to the review's session key as soon as the session loads, or two
// live or preview reviews that reuse a port restore and delete each other's.
test('live mode scopes drafts to the review right after the session loads', () => {
  const src = fs.readFileSync(path.join(__dirname, '..', 'live-mode.js'), 'utf8');
  const load = src.indexOf('state.session = await waitForSession();');
  assert.ok(load > 0, 'session load not found');
  const scope = src.indexOf('draft.setScope(state.session.session_key)', load);
  assert.ok(scope > load, 'live mode must call draft.setScope(state.session.session_key) after loading the session');
  const firstDraftUse = src.indexOf("draftMod.loadDraft('live-", 0);
  assert.ok(scope < firstDraftUse || firstDraftUse < 0, 'the scope must be set before any draft is read');
});
