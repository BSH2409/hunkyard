'use strict';
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.live = root.hunkyard.live || {};
    root.hunkyard.live.toggle = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {
  function reduceToggle(mode) { return mode === 'pin' ? 'navigate' : 'pin'; }
  return { reduceToggle };
});
