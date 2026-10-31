// hunkyard-reload.js — one page reload at a time. Starting a reload with new
// inputs aborts the one in flight; every caller's promise settles when the
// latest reload finishes, so code awaiting an earlier click still sees the
// final state. A reload with the same inputs joins the one in flight.
// Depends on no other window.hunkyard namespace.
(function () {
  'use strict';

  // createReloader(run): run(signal, key) does the work and must not touch
  // the page once signal.aborted is true. Returns { start(key), busy(),
  // whenIdle() }.
  function createReloader(run) {
    var current = null;
    var currentKey = null;
    var controller = null;

    function waitLatest() {
      var p = current;
      return p.then(function () {
        return p === current ? undefined : waitLatest();
      }, function (err) {
        if (p !== current) return waitLatest();
        throw err;
      });
    }

    function start(key) {
      if (current && key === currentKey) return waitLatest();
      if (controller) controller.abort();
      var mine = new AbortController();
      controller = mine;
      currentKey = key;
      var run$ = Promise.resolve().then(function () { return run(mine.signal, key); }).catch(function (err) {
        if (!mine.signal.aborted) throw err;
      });
      current = run$.then(function () {
        if (controller === mine) { controller = null; currentKey = null; }
      }, function (err) {
        if (controller === mine) { controller = null; currentKey = null; }
        throw err;
      });
      return waitLatest();
    }

    function busy() { return !!controller; }

    // whenIdle settles once no reload is running (errors are swallowed: the
    // caller only wants to act after the page has settled).
    function whenIdle() {
      if (!controller || !current) return Promise.resolve();
      return waitLatest().catch(function () {});
    }

    return { start: start, busy: busy, whenIdle: whenIdle };
  }

  var api = { createReloader: createReloader };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.reload = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
