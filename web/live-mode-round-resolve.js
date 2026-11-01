'use strict';
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.live = root.hunkyard.live || {};
    root.hunkyard.live.roundResolve = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {
  function pinsToResolveAtRoundStart(pins, currentPath) {
    const out = [];
    for (const p of pins || []) {
      if (!p || !p.dom_anchor) continue;
      if (p.dom_anchor.pathname !== currentPath) continue;
      if (p._roundResolved) continue;
      out.push(p.id);
    }
    return out;
  }
  // The status the page found for a pin, when the server has not recorded
  // it for this round yet; null when there is nothing to report.
  function pinStatusToReport(pin, msg, round) {
    if (!pin || !msg || !msg.pin_status) return null;
    if (pin.pin_status === msg.pin_status && pin.pin_checked_round === round) return null;
    return msg.pin_status;
  }
  return { pinsToResolveAtRoundStart, pinStatusToReport };
});
