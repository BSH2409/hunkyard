'use strict';
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.live = root.hunkyard.live || {};
    root.hunkyard.live.origin = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {
  function makeOriginGuard(opts) {
    var expectSource = opts.expectSource;
    var expectOrigin = opts.expectOrigin;
    return function check(ev) {
      if (ev.source !== expectSource) return false;
      if (ev.origin !== expectOrigin) return false;
      return true;
    };
  }
  return { makeOriginGuard };
});
