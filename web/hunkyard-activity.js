// hunkyard-activity.js — tells the daemon someone is using the review, so an
// open tab with nobody at it does not keep the review from pausing. Reports
// only real interaction, at most once per throttle window.
// Depends on no other window.hunkyard namespace.
(function () {
  'use strict';

  var EVENTS = ['pointerdown', 'keydown', 'wheel', 'scroll', 'focusin'];

  // start({ post, throttleMs, target, now }): post() sends the report
  // (default: POST /api/activity). Returns { stop(), mark() }; mark() reports
  // interaction seen elsewhere (the live/preview iframe) under the same
  // throttle.
  function start(opts) {
    var o = opts || {};
    var target = o.target || (typeof document !== 'undefined' ? document : null);
    var now = o.now || function () { return Date.now(); };
    var throttleMs = typeof o.throttleMs === 'number' ? o.throttleMs : 60000;
    var post = o.post || function () {
      fetch('/api/activity', { method: 'POST' }).catch(function () {});
    };
    var last = -Infinity;
    function onActivity() {
      var t = now();
      if (t - last < throttleMs) return;
      last = t;
      post();
    }
    if (target) {
      EVENTS.forEach(function (type) { target.addEventListener(type, onActivity, { capture: true, passive: true }); });
    }
    return {
      mark: onActivity,
      stop: function () {
        if (!target) return;
        EVENTS.forEach(function (type) { target.removeEventListener(type, onActivity, { capture: true }); });
      },
    };
  }

  var api = { start: start };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.activity = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
