// live-mode-pin-shot.js — keeps the screenshot the frame captured for the pin
// being composed, uploads it, and builds the pin's screenshot fields.
// Depends on no other window.hunkyard namespace.
(function () {
  'use strict';

  // Shots are kept per selection, so a save still waiting for its own shot
  // is not handed the shot of an element picked after it.
  function createShotStore() {
    var entries = new Map();

    return {
      open: function (selectionId) {
        if (selectionId && !entries.has(selectionId)) entries.set(selectionId, { shot: null, waiters: [] });
      },
      receive: function (msg) {
        var entry = msg && entries.get(msg.selection_id);
        if (!entry || entry.shot) return;
        entry.shot = msg;
        var waiters = entry.waiters;
        entry.waiters = [];
        waiters.forEach(function (resolve) { resolve(msg); });
      },
      close: function (selectionId) {
        var entry = entries.get(selectionId);
        if (!entry) return;
        entries.delete(selectionId);
        entry.waiters.forEach(function (resolve) { resolve(null); });
      },
      waitFor: function (selectionId, timeoutMs) {
        var entry = entries.get(selectionId);
        if (!entry || entry.shot) return Promise.resolve(entry ? entry.shot : null);
        return new Promise(function (resolve) {
          var timer = setTimeout(function () {
            entry.waiters = entry.waiters.filter(function (w) { return w !== done; });
            resolve(null);
          }, timeoutMs);
          function done(value) {
            clearTimeout(timer);
            resolve(value);
          }
          entry.waiters.push(done);
        });
      },
    };
  }

  function pinShotPayload(shot, upload) {
    if (!shot) return { screenshot_status: 'failed', screenshot_reason: 'timeout' };
    if (!shot.ok) return { screenshot_status: 'failed', screenshot_reason: shot.reason || 'unknown' };
    return {
      pin_screenshot: {
        file: upload.filename,
        width: shot.width,
        height: shot.height,
        scale: shot.scale,
        band_offset_y: shot.band_offset_y,
        marker_rect: shot.marker_rect,
        viewport: shot.viewport,
      },
    };
  }

  async function uploadShot(shot, fetchFn) {
    var form = new FormData();
    form.append('file', new Blob([shot.bytes], { type: 'image/png' }), 'pin.png');
    var res = await fetchFn('/api/attachments', { method: 'POST', body: form });
    if (!res.ok) throw new Error('screenshot upload failed: ' + res.status + ' ' + (await res.text()));
    return res.json();
  }

  var api = { createShotStore: createShotStore, pinShotPayload: pinShotPayload, uploadShot: uploadShot };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.live = window.hunkyard.live || {};
    window.hunkyard.live.pinShot = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
