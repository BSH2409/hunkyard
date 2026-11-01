'use strict';
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.live = root.hunkyard.live || {};
    root.hunkyard.live.reanchorPut = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {
  // shotFields: the screenshot fields for the new element (pin_screenshot,
  // or screenshot_status/screenshot_reason when capture failed).
  function buildReanchorRequest(pinId, domAnchor, shotFields) {
    return {
      method: 'PUT',
      url: `/api/comment/${encodeURIComponent(pinId)}?path=${encodeURIComponent(domAnchor.pathname)}`,
      body: JSON.stringify(Object.assign({ dom_anchor: domAnchor }, shotFields || {})),
      headers: { 'Content-Type': 'application/json' },
    };
  }
  return { buildReanchorRequest };
});
