const { describe, it, beforeEach } = require('node:test');
const assert = require('node:assert/strict');

// Mock EventSource
class MockEventSource {
  constructor(url) {
    this.url = url;
    this.listeners = {};
    this.onopen = null;
    this.onerror = null;
    this.closed = false;
    MockEventSource.last = this;
  }
  addEventListener(type, handler) {
    if (!this.listeners[type]) this.listeners[type] = [];
    this.listeners[type].push(handler);
  }
  close() { this.closed = true; }
  emit(type, data) {
    var handlers = this.listeners[type] || [];
    handlers.forEach(function (h) { h({ data: data }); });
  }
}
global.EventSource = MockEventSource;

const sse = require('../hunkyard-sse.js');

describe('hunkyard-sse', () => {
  beforeEach(() => { MockEventSource.last = null; });

  it('creates EventSource with correct URL', () => {
    sse.createSSE('/api/events', {});
    assert.equal(MockEventSource.last.url, '/api/events');
  });

  it('registers event handlers', () => {
    var received = null;
    sse.createSSE('/api/events', {
      'file-changed': function (data) { received = data; },
    });
    MockEventSource.last.emit('file-changed', '{"round":2}');
    assert.deepEqual(received, { round: 2 });
  });

  it('passes raw string if JSON parse fails', () => {
    var received = null;
    sse.createSSE('/api/events', {
      'ping': function (data) { received = data; },
    });
    MockEventSource.last.emit('ping', 'not-json');
    assert.equal(received, 'not-json');
  });

  it('close() stops the EventSource', () => {
    var conn = sse.createSSE('/api/events', {});
    assert.equal(MockEventSource.last.closed, false);
    conn.close();
    assert.equal(MockEventSource.last.closed, true);
  });

  it('calls onOpen callback', () => {
    var opened = false;
    sse.createSSE('/api/events', {}, { onOpen: function () { opened = true; } });
    MockEventSource.last.onopen();
    assert.equal(opened, true);
  });
});

it('coalesced runs one refresh at a time and one follow-up for any events that arrived meanwhile', async () => {
  const { coalesced } = require('../hunkyard-sse.js');
  let running = 0, maxRunning = 0, runs = 0;
  const releases = [];
  const refresh = coalesced(() => new Promise((resolve) => {
    runs++; running++; maxRunning = Math.max(maxRunning, running);
    releases.push(() => { running--; resolve(); });
  }));
  const first = refresh();
  refresh();
  refresh();
  assert.equal(runs, 1, 'second and third events wait for the running refresh');
  releases.shift()();
  await first;
  await new Promise((r) => setImmediate(r));
  assert.equal(runs, 2, 'one follow-up covers both queued events');
  releases.shift()();
  await new Promise((r) => setImmediate(r));
  assert.equal(runs, 2);
  assert.equal(maxRunning, 1);
});
