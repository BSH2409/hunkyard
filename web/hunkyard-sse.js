(function () {
  'use strict';

  /**
   * createSSE(url, handlers, opts) → { close, source }
   *
   * Thin wrapper around EventSource that:
   * - Registers typed event listeners from a handler map
   * - Parses JSON data for each event (passes raw string on parse failure)
   * - Provides a close() method for cleanup
   *
   * EventSource handles reconnection natively — no custom retry logic needed.
   *
   * handlers: { 'event-name': function(data) { ... }, ... }
   * opts.onOpen:  called when connection opens
   * opts.onError: called on connection error (before native retry)
   */
  function createSSE(url, handlers, opts) {
    opts = opts || {};
    var source = new EventSource(url);
    var closed = false;

    if (typeof opts.onOpen === 'function') {
      source.onopen = opts.onOpen;
    }
    if (typeof opts.onError === 'function') {
      source.onerror = function (e) {
        if (!closed) opts.onError(e);
      };
    }

    var types = Object.keys(handlers);
    for (var i = 0; i < types.length; i++) {
      (function (type) {
        source.addEventListener(type, function (event) {
          var data = event.data;
          try { data = JSON.parse(event.data); } catch (_) {}
          handlers[type](data);
        });
      })(types[i]);
    }

    return {
      close: function () {
        closed = true;
        source.close();
      },
      source: source,
    };
  }

  // coalesced wraps an async refresh so events never overlap it: one run at a
  // time, and events that arrive meanwhile share a single follow-up run.
  function coalesced(run) {
    var inFlight = null;
    var queued = false;
    function start() {
      var running;
      try {
        running = Promise.resolve(run());
      } catch (err) {
        running = Promise.reject(err);
      }
      inFlight = running.finally(function () {
        inFlight = null;
        if (queued) {
          queued = false;
          start();
        }
      });
      return inFlight;
    }
    return function () {
      if (inFlight) {
        queued = true;
        return inFlight;
      }
      return start();
    };
  }

  var api = { createSSE: createSSE, coalesced: coalesced };

  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.sse = api;
  }

  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
