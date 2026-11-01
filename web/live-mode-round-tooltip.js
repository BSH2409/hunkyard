'use strict';
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.live = root.hunkyard.live || {};
    root.hunkyard.live.roundTooltip = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {
  function composeRoundTooltip({ round, pins } = {}) {
    let resolved = 0, outdated = 0, unchecked = 0;
    const list = pins || [];
    for (const p of list) {
      if (!p) continue;
      if (p.resolved) { resolved++; continue; }
      if ((p.pin_checked_round || 0) < round) unchecked++;
      else if (p.pin_status === 'outdated') outdated++;
    }
    const total = list.length;
    const carried = total - resolved;
    return { round, total, resolved, outdated, unchecked, carried };
  }
  return { composeRoundTooltip };
});
