'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

test('publication retries keep their identity until the review data changes', async () => {
  const form = new EventTarget();
  const dialog = new EventTarget(); dialog.querySelector = () => form; dialog.showModal = () => {}; dialog.close = () => {};
  const summary = { value: 'Please check this boundary.' };
  const elements = { publishReviewDialog: dialog, publishSummary: summary, publishReviewSubmit: {}, publishReviewCancel: new EventTarget() };
  summary.focus = () => {};
  let nextID = 0;
  const context = { window: {}, document: { getElementById: id => elements[id], activeElement: null }, crypto: { randomUUID: () => `request-${++nextID}` }, FormData: class { get() { return 'comment'; } } };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../hunkyard-review.js'), 'utf8'), context);
  const publisher = context.window.hunkyardReview;
  let draft = 'human revision one';
  const publish = async () => { const pending = publisher.prepare(() => draft); form.dispatchEvent(new Event('submit', { cancelable: true })); return pending; };
  const first = await publish();
  const retry = await publish();
  assert.equal(retry.request_id, first.request_id);
  draft = 'human revision two';
  const changed = await publish();
  assert.notEqual(changed.request_id, first.request_id);
  publisher.published();
  assert.notEqual((await publish()).request_id, changed.request_id);
});
