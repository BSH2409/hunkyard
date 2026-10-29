'use strict';
// Regression: editing a comment with replies nests .comment-replies inside
// .comment-form. Replies no longer inherit body font from .comment-card, so
// .reply-body must declare it explicitly (matching .comment-body).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

function replyBodyRule(css) {
  const match = css.match(/\.reply-body\s*\{[^}]+\}/);
  assert.ok(match, 'expected a .reply-body rule');
  return match[0];
}

function assertReplyBodyUsesBodyFont(css, label) {
  const rule = replyBodyRule(css);
  assert.match(
    rule,
    /font-family:\s*var\(--hunkyard-font-body\)/,
    `${label}: .reply-body must set body font so replies stay proportional when nested in .comment-form during edit`
  );
}

test('style.css: .reply-body declares body font', () => {
  const css = fs.readFileSync(path.join(__dirname, '../style.css'), 'utf8');
  assertReplyBodyUsesBodyFont(css, 'hunkyard/web/style.css');
});
