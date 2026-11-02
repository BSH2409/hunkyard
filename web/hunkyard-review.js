(function () {
  'use strict';
  var dialog = document.getElementById('publishReviewDialog');
  if (!dialog) return;
  var form = dialog.querySelector('form');
  var summary = document.getElementById('publishSummary');
  var submit = document.getElementById('publishReviewSubmit');
  var pending = null;
  var request = null;
  var fingerprint = '';
  // crypto.randomUUID exists only on secure origins (localhost, HTTPS); a
  // review opened on a LAN or container address builds a v4 UUID itself.
  function newRequestID() {
    if (typeof crypto.randomUUID === 'function') return crypto.randomUUID();
    var b = crypto.getRandomValues(new Uint8Array(16));
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    var hex = Array.prototype.map.call(b, function (x) { return (x + 0x100).toString(16).slice(1); }).join('');
    return hex.slice(0, 8) + '-' + hex.slice(8, 12) + '-' + hex.slice(12, 16) + '-' + hex.slice(16, 20) + '-' + hex.slice(20);
  }
  window.hunkyardReview = {
    prepare: function (publicationKey) {
      if (pending) return pending;
      pending = (async function () {
      if (window.hunkyardSkills) await window.hunkyardSkills.render(document.getElementById('publishSkills'), window.fetch.bind(window));
      return new Promise(function (resolve) {
        var previousFocus = document.activeElement;
        function close(value) {
          form.removeEventListener('submit', publish);
          dialog.removeEventListener('cancel', cancel);
          document.getElementById('publishReviewCancel').removeEventListener('click', cancel);
          dialog.close();
          pending = null;
          if (previousFocus) previousFocus.focus();
          resolve(value);
        }
        function cancel(event) { event.preventDefault(); close(false); }
        function publish(event) {
          event.preventDefault();
          var value = { disposition: new FormData(form).get('disposition'), summary: summary.value, skill_ids: window.hunkyardSkills ? window.hunkyardSkills.selection(document.getElementById('publishSkills')) : [] };
          var context = typeof publicationKey === 'function' ? publicationKey() : null;
          if (context && context.displayed_files) value.displayed_files = context.displayed_files;
          var nextFingerprint = JSON.stringify([value, context]);
          if (!request || fingerprint !== nextFingerprint) {
            request = newRequestID();
            fingerprint = nextFingerprint;
          }
          value.request_id = request;
          close(value);
        }
        form.addEventListener('submit', publish);
        dialog.addEventListener('cancel', cancel);
        document.getElementById('publishReviewCancel').addEventListener('click', cancel);
        submit.disabled = false;
        var blockingComments = typeof window.hunkyardOpenBlockingComments === 'function' ? window.hunkyardOpenBlockingComments() : 0;
        var blockingDecisions = typeof window.hunkyardOpenBlockingDecisions === 'function' ? window.hunkyardOpenBlockingDecisions() : 0;
        var blocking = blockingComments + blockingDecisions;
        var approve = form.querySelector('input[name="disposition"][value="approve"]');
        var approveHint = document.getElementById('publishApproveBlocked');
        if (approve) {
          approve.disabled = blocking > 0;
          if (approve.disabled && approve.checked) form.querySelector('input[name="disposition"][value="comment"]').checked = true;
        }
        if (approveHint) {
          approveHint.hidden = blocking === 0;
          var reasons = [];
          if (blockingComments) reasons.push(blockingComments === 1 ? '1 blocking comment is unresolved.' : blockingComments + ' blocking comments are unresolved.');
          if (blockingDecisions) reasons.push(blockingDecisions === 1 ? '1 blocking decision is unanswered.' : blockingDecisions + ' blocking decisions are unanswered.');
          approveHint.textContent = reasons.join(' ');
        }
        dialog.showModal();
        summary.focus();
      });
      }()).catch(function (error) { pending = null; throw error; });
      return pending;
    },
    published: function () { request = null; fingerprint = ''; summary.value = ''; },
  };
}());
