'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createReloader } = require('../hunkyard-reload.js');

function deferred() {
  let resolve;
  const promise = new Promise(function (r) { resolve = r; });
  return { promise, resolve };
}

test('a new key aborts the previous run', async () => {
  const signals = [];
  const gates = [deferred(), deferred()];
  const reloader = createReloader(function (signal) {
    signals.push(signal);
    return gates[signals.length - 1].promise;
  });
  const first = reloader.start('a');
  const second = reloader.start('b');
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(signals[0].aborted, true);
  assert.equal(signals[1].aborted, false);
  gates[0].resolve();
  gates[1].resolve();
  await Promise.all([first, second]);
});

test('every caller waits for the latest run', async () => {
  const order = [];
  const gates = [deferred(), deferred()];
  let n = 0;
  const reloader = createReloader(function () {
    const gate = gates[n++];
    return gate.promise;
  });
  const first = reloader.start('a').then(function () { order.push('first'); });
  const second = reloader.start('b').then(function () { order.push('second'); });
  gates[0].resolve();
  await new Promise(function (r) { setTimeout(r, 5); });
  assert.deepEqual(order, [], 'the superseded run finishing must not release callers');
  gates[1].resolve();
  await Promise.all([first, second]);
  assert.deepEqual(order.sort(), ['first', 'second']);
});

test('the same key joins the run in flight', async () => {
  let runs = 0;
  const gate = deferred();
  const reloader = createReloader(function () { runs++; return gate.promise; });
  const a = reloader.start('same');
  const b = reloader.start('same');
  gate.resolve();
  await Promise.all([a, b]);
  assert.equal(runs, 1);
});

test('an aborted run does not apply its result', async () => {
  const applied = [];
  const gates = [deferred(), deferred()];
  let n = 0;
  const reloader = createReloader(async function (signal, key) {
    await gates[n++].promise;
    if (signal.aborted) return;
    applied.push(key);
  });
  const first = reloader.start('old');
  const second = reloader.start('new');
  gates[0].resolve();
  gates[1].resolve();
  await Promise.all([first, second]);
  assert.deepEqual(applied, ['new']);
});

test('a failure in the latest run reaches its callers', async () => {
  const reloader = createReloader(function () { return Promise.reject(new Error('boom')); });
  await assert.rejects(reloader.start('x'), /boom/);
});

test('a caller joining a finished key starts a new run', async () => {
  let runs = 0;
  const reloader = createReloader(function () { runs++; return Promise.resolve(); });
  await reloader.start('k');
  await reloader.start('k');
  assert.equal(runs, 2);
});

test('whenIdle waits for the latest run and is immediate when idle', async () => {
  const gate = deferred();
  const reloader = createReloader(function () { return gate.promise; });
  await reloader.whenIdle();
  let idle = false;
  reloader.start('k');
  const waiting = reloader.whenIdle().then(function () { idle = true; });
  await Promise.resolve();
  assert.equal(idle, false);
  gate.resolve();
  await waiting;
  assert.equal(idle, true);
});
