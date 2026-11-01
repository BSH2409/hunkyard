'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createShotStore, pinShotPayload, uploadShot } = require('../live-mode-pin-shot.js');

const okShot = (id) => ({ selection_id: id, ok: true, bytes: new ArrayBuffer(4), width: 1280, height: 2000, scale: 1, band_offset_y: 0, marker_rect: { x: 1, y: 2, width: 3, height: 4 }, viewport: { width: 1280, height: 800, scroll_x: 0, scroll_y: 0 } });

test('the payload carries the uploaded file and the capture metadata', () => {
  assert.deepEqual(pinShotPayload(okShot('s1'), { filename: 'a1b2c3d4-0000-4000-8000-000000000000.png' }), {
    pin_screenshot: { file: 'a1b2c3d4-0000-4000-8000-000000000000.png', width: 1280, height: 2000, scale: 1, band_offset_y: 0, marker_rect: { x: 1, y: 2, width: 3, height: 4 }, viewport: { width: 1280, height: 800, scroll_x: 0, scroll_y: 0 } },
  });
});

test('a failed capture is saved as a failed screenshot with its reason', () => {
  assert.deepEqual(pinShotPayload({ selection_id: 's1', ok: false, reason: 'blank' }, null), { screenshot_status: 'failed', screenshot_reason: 'blank' });
  assert.deepEqual(pinShotPayload(null, null), { screenshot_status: 'failed', screenshot_reason: 'timeout' });
});

test('the store keeps each selection shot apart', async () => {
  const store = createShotStore();
  store.open('s2');
  store.receive(okShot('s1'));
  assert.equal(await store.waitFor('s2', 10), null, 'a shot for another selection is ignored');
  store.receive(okShot('s2'));
  assert.equal((await store.waitFor('s2', 10)).selection_id, 's2');
});

test('a shot arriving after its selection closed is ignored', async () => {
  const store = createShotStore();
  store.open('s3');
  store.close('s3');
  store.receive(okShot('s3'));
  assert.equal(await store.waitFor('s3', 10), null);
});

test('waitFor resolves when the shot arrives during the wait', async () => {
  const store = createShotStore();
  store.open('s4');
  const waiting = store.waitFor('s4', 1000);
  setTimeout(() => store.receive(okShot('s4')), 5);
  assert.equal((await waiting).selection_id, 's4');
});

test('a pending save keeps its own shot when another element is picked', async () => {
  const store = createShotStore();
  store.open('a');
  const saving = store.waitFor('a', 1000);
  store.open('b');
  store.receive(okShot('b'));
  store.receive(okShot('a'));
  assert.equal((await saving).selection_id, 'a');
  assert.equal((await store.waitFor('b', 10)).selection_id, 'b');
});

test('uploadShot posts the PNG as multipart "file" and checks the response', async () => {
  const seen = [];
  const fetchOk = async (url, init) => { seen.push([url, init]); return { ok: true, json: async () => ({ filename: 'f.png' }) }; };
  assert.deepEqual(await uploadShot(okShot('s5'), fetchOk), { filename: 'f.png' });
  assert.equal(seen[0][0], '/api/attachments');
  assert.equal(seen[0][1].method, 'POST');
  const file = seen[0][1].body.get('file');
  assert.equal(file.type, 'image/png');
  const fetchFail = async () => ({ ok: false, status: 413, text: async () => 'too large' });
  await assert.rejects(() => uploadShot(okShot('s5'), fetchFail), /413/);
});
