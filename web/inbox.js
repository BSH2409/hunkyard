// Review inbox page (/inbox): every running and paused review on this
// machine, refreshed every few seconds. Reads /api/inbox; resumes a paused
// review with POST /api/inbox/resume.
(function () {
  'use strict';

  var STATES = {
    waiting_for_reviewer: 'Waiting for you',
    agent_working: 'Agent working',
    paused: 'Paused',
    unreachable: 'Not answering',
  };

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function rowHTML(e) {
    var action = e.state === 'paused'
      ? '<button type="button" class="btn btn-sm" data-inbox-resume="' + esc(e.key) + '">Resume</button>'
      : (e.url ? '<a class="btn btn-sm" href="' + esc(e.url) + '" target="_blank" rel="noopener">Open</a>' : '');
    return '<li class="inbox-row inbox-row--' + esc(e.state) + '">' +
      '<span class="inbox-state">' + esc(STATES[e.state] || e.state) + '</span>' +
      '<span class="inbox-main"><span class="inbox-label">' + esc(e.label) + '</span>' +
      '<span class="inbox-meta">' + esc(e.repository || e.cwd || '') + ' · round ' + (e.round || 0) + ' · ' + (e.unresolved || 0) + ' open</span></span>' +
      action + '</li>';
  }

  function listHTML(entries) {
    if (!entries || !entries.length) return '<p class="inbox-empty">No reviews are running or paused.</p>';
    return '<ul class="inbox-list">' + entries.map(rowHTML).join('') + '</ul>';
  }

  function summary(entries) {
    var counts = {};
    (entries || []).forEach(function (e) { counts[e.state] = (counts[e.state] || 0) + 1; });
    var parts = [];
    if (counts.waiting_for_reviewer) parts.push(counts.waiting_for_reviewer + ' waiting for you');
    if (counts.agent_working) parts.push(counts.agent_working + ' agent working');
    if (counts.paused) parts.push(counts.paused + ' paused');
    return parts.join(' · ');
  }

  // start renders into root and polls; it returns a stop function.
  function start(root, intervalMs) {
    var list = root.querySelector('#inboxList');
    var status = root.querySelector('#inboxSummary');
    var inFlight = null;
    function refresh() {
      if (inFlight) return inFlight;
      inFlight = fetch('/api/inbox').then(function (res) {
        if (!res.ok) throw new Error('Server returned ' + res.status);
        return res.json();
      }).then(function (entries) {
        list.innerHTML = listHTML(entries);
        status.textContent = summary(entries);
      }).catch(function () {
        status.textContent = 'Could not load the inbox.';
      }).finally(function () { inFlight = null; });
      return inFlight;
    }
    list.addEventListener('click', function (event) {
      var button = event.target.closest('[data-inbox-resume]');
      if (!button) return;
      button.disabled = true;
      button.textContent = 'Resuming…';
      fetch('/api/inbox/resume', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ key: button.getAttribute('data-inbox-resume') }) })
        .then(function (res) {
          if (!res.ok) return res.text().then(function (text) { throw new Error(text || res.statusText); });
          return refresh();
        })
        .catch(function (err) {
          button.disabled = false;
          button.textContent = 'Resume';
          status.textContent = 'Could not resume: ' + err.message;
        });
    });
    refresh();
    var timer = setInterval(function () { if (!document.hidden) refresh(); }, intervalMs || 5000);
    return function () { clearInterval(timer); };
  }

  var api = { listHTML: listHTML, summary: summary, start: start };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.inbox = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
