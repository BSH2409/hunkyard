'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const decisions = require('../hunkyard-decisions.js');

const first = { id: 'd_1', question: 'Which <auth>?', options: ['Session', 'JWT'], status: 'open', blocking: true, context: { path: 'a.go', line: 3 }, asked_at: '2026-11-01T10:00:00Z' };
const second = { id: 'd_2', question: 'Name it?', options: [], allow_other: true, status: 'open', asked_at: '2026-11-01T10:05:00Z' };
const answered = { id: 'd_3', question: 'Keep it?', options: ['yes', 'no'], status: 'answered', chosen: 'yes', text: 'for now', answered_at: '2026-11-01T10:10:00Z', asked_at: '2026-11-01T09:00:00Z' };
const dismissed = { id: 'd_4', question: 'Rename?', options: ['yes', 'no'], status: 'dismissed', asked_at: '2026-11-01T08:00:00Z' };

test('openBlockingCount and summaryLine count open questions', () => {
  assert.equal(decisions.openBlockingCount([first, second, answered]), 1);
  assert.equal(decisions.summaryLine([first, second, answered]), '2 decisions are unanswered.');
  assert.equal(decisions.summaryLine([answered]), '');
});

test('pages are the open questions, oldest first', () => {
  assert.deepEqual(decisions.pages([second, answered, first, dismissed]).map((d) => d.id), ['d_1', 'd_2']);
});

test('a question page shows progress, options and a custom answer', () => {
  const html = decisions.questionPageHTML(first, {}, 0, 2);
  assert.match(html, /Question 1 of 2/);
  assert.match(html, /Which &lt;auth&gt;\?/);
  assert.equal((html.match(/data-decision-choice=/g) || []).length, 2);
  assert.match(html, /Other/);
  assert.match(html, /Blocks approval/);
  assert.match(html, /data-decision-context="a\.go:3"/);
  assert.match(html, /data-decision-dismiss/);
  assert.match(decisions.questionPageHTML(first, { chosen: 'JWT' }, 0, 2), /aria-checked="true"[^>]*data-decision-choice="JWT"|data-decision-choice="JWT"[^>]*aria-checked="true"/);
  assert.match(decisions.questionPageHTML(second, {}, 1, 2), /Question 2 of 2/);
});

test('answerText describes a draft or a published answer', () => {
  assert.equal(decisions.answerText({ chosen: 'JWT' }), 'JWT');
  assert.equal(decisions.answerText({ text: 'auth_v2' }), 'auth_v2');
  assert.equal(decisions.answerText({ dismiss: true }), 'Dismissed');
  assert.equal(decisions.answerText({}), '');
  assert.equal(decisions.answerText(answered), 'yes — for now');
  assert.equal(decisions.answerText(dismissed), 'Dismissed');
});

test('the review page lists every question with the draft answer and a way back', () => {
  const html = decisions.reviewPageHTML([first, second], { d_1: { chosen: 'JWT' } });
  assert.match(html, /Which &lt;auth&gt;\?[^]*JWT/);
  assert.match(html, /Name it\?[^]*No answer yet/);
  assert.equal((html.match(/data-decision-goto=/g) || []).length, 2);
  assert.match(html, /data-decision-submit/);
  assert.match(html, /1 of 2 answered/);
});

test('answersToSubmit sends only completed drafts of open questions', () => {
  const out = decisions.answersToSubmit([first, second, answered], { d_1: { chosen: 'JWT' }, d_2: { text: '  ' }, d_3: { chosen: 'no' } });
  assert.deepEqual(out, [{ id: 'd_1', answer: { chosen: 'JWT' } }]);
});

test('after a submit, the done page confirms what was sent and folds earlier answers away', () => {
  const html = decisions.historyPageHTML([answered, dismissed], ['d_3']);
  assert.match(html, /Sent to the agent/);
  const sent = html.slice(0, html.indexOf('<details'));
  assert.match(sent, /Keep it\?[^]*yes — for now/);
  assert.doesNotMatch(sent, /Rename\?/);
  assert.match(html, /<details[^>]*>[^]*Earlier answers[^]*Rename\?[^]*Dismissed/);
  assert.doesNotMatch(html, /<details[^>]* open/);
});

test('without a fresh submit, the done page lists earlier answers under a heading', () => {
  const html = decisions.historyPageHTML([answered, dismissed], []);
  assert.doesNotMatch(html, /Sent to the agent/);
  assert.doesNotMatch(html, /<details/);
  assert.match(html, /Earlier answers[^]*Keep it\?[^]*Rename\?/);
});

test('a submit that leaves nothing earlier has no earlier section', () => {
  assert.doesNotMatch(decisions.historyPageHTML([answered], ['d_3']), /Earlier answers/);
});

test('the dock opens for questions it has not shown yet', () => {
  assert.deepEqual(decisions.dockState([first], [], 'closed'), { state: 'open', seen: ['d_1'] });
  assert.deepEqual(decisions.dockState([first], ['d_1'], 'closed'), { state: 'closed', seen: ['d_1'] });
  assert.equal(decisions.dockState([], [], 'open').state, 'hidden');
});
