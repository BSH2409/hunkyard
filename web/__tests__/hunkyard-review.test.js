'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

test('publication retries keep their identity until the review data changes', async () => {
  const form = new EventTarget(); form.querySelector = () => null;
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

test('publishing works on a plain-HTTP LAN address, where crypto.randomUUID is missing', async () => {
  const form = new EventTarget(); form.querySelector = () => null;
  const dialog = new EventTarget(); dialog.querySelector = () => form; dialog.showModal = () => {}; dialog.close = () => {};
  const summary = { value: '', focus: () => {} };
  const elements = { publishReviewDialog: dialog, publishSummary: summary, publishReviewSubmit: {}, publishReviewCancel: new EventTarget() };
  let counter = 0;
  const crypto = { getRandomValues: (bytes) => { for (let i = 0; i < bytes.length; i++) bytes[i] = (counter++ * 37) & 255; return bytes; } };
  const context = { window: {}, document: { getElementById: id => elements[id], activeElement: null }, crypto, Uint8Array, FormData: class { get() { return 'approve'; } } };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../hunkyard-review.js'), 'utf8'), context);
  const pending = context.window.hunkyardReview.prepare(() => null);
  form.dispatchEvent(new Event('submit', { cancelable: true }));
  const value = await pending;
  assert.match(value.request_id, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
});
