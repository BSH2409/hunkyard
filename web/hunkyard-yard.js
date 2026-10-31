// hunkyard-yard.js — the Preference Yard: what learn-preferences loads,
// learns and blocks. Rendered into the settings overlay's Yard tab and into
// the standalone /yard page (yard.html).
//
// Reads: /api/yard/{rules,persona,pipeline,loads,activity,health}; writes (same-origin
// POST): /api/yard/rules, /api/yard/rules/{id}/{retire,promote}, /api/yard/maintenance.
// Depends on no other window.hunkyard namespace.
(function () {
  'use strict';

  var VIEWS = [
    { id: 'review', label: 'This review', modes: ['tab'] },
    { id: 'rules', label: 'Rules' },
    { id: 'pipeline', label: 'Pipeline' },
    { id: 'persona', label: 'Persona' },
    { id: 'loads', label: 'Loads' },
    { id: 'activity', label: 'Activity' },
    { id: 'health', label: 'Health' },
  ];

  var EVENT_LABELS = {
    captured: 'captured', learned: 'learned', confirmed: 'confirmed', violated: 'violated again',
    blocked: 'blocked', overridden: 'allowed once', interrupted: 'interrupted',
    maintenance_succeeded: 'maintenance ran', maintenance_failed: 'maintenance failed',
  };

  function esc(text) {
    return String(text == null ? '' : text).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function repoName(key) {
    return String(key || '').replace(/-[0-9a-f]{12}$/, '');
  }

  function shortTime(iso) {
    var date = new Date(iso);
    return isNaN(date) ? '' : date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }

  function empty(message) {
    return '<div class="yard-empty">' + esc(message) + '</div>';
  }

  function isActive(rule) {
    return rule.status === 'active' || rule.status === 'core';
  }

  function filterRules(rules, filter) {
    filter = filter || {};
    var status = filter.status || 'active';
    return (rules || []).filter(function (rule) {
      if (status === 'active' && !isActive(rule)) return false;
      if (status !== 'active' && status !== 'all' && rule.status !== status) return false;
      if (filter.scope && rule.scope_kind !== filter.scope) return false;
      if (filter.surface && (rule.surfaces || []).indexOf(filter.surface) === -1) return false;
      return true;
    });
  }

  function rulesHTML(rules, filter) {
    var visible = filterRules(rules, filter);
    if (!visible.length) return empty('No preferences yet. Corrections and review comments you make become rules here.');
    var html = '<ul class="yard-list">';
    visible.forEach(function (rule) {
      var scope = rule.scope_kind === 'global'
        ? '<span class="yard-label yard-label--global">global</span>'
        : '<span class="yard-label yard-label--repo">' + esc(repoName(rule.repo_key)) + '</span>';
      var surfaces = (rule.surfaces || []).map(function (s) { return '<span class="yard-label">' + esc(s) + '</span>'; }).join('');
      var violations = rule.violations ? '<span class="yard-label yard-label--violated">' + rule.violations + ' violation' + (rule.violations === 1 ? '' : 's') + '</span>' : '';
      var actions = '';
      if (isActive(rule)) {
        if (rule.scope_kind === 'repository') actions += '<button type="button" class="yard-btn" data-yard-action="promote" data-rule-id="' + esc(rule.id) + '">Make global</button>';
        actions += '<button type="button" class="yard-btn" data-yard-action="retire" data-rule-id="' + esc(rule.id) + '">Retire</button>';
      }
      html += '<li class="yard-row"><span class="yard-dot yard-dot--' + esc(rule.status) + '" aria-label="' + esc(rule.status) + '"></span>' +
        '<div class="yard-row-main"><div class="yard-row-title">' + esc(rule.directive) + scope + surfaces + violations + '</div>' +
        '<div class="yard-row-meta">' + esc(rule.status) + ' · ' + rule.observations + ' observation' + (rule.observations === 1 ? '' : 's') + ' · ' + esc(rule.origin) + ' · updated ' + esc(shortTime(rule.updated_at)) + '</div></div>' +
        '<div class="yard-row-actions">' + actions + '</div></li>';
    });
    return html + '</ul>';
  }

  function pipelineHTML(pipeline) {
    pipeline = pipeline || {};
    var captured = pipeline.captured || [];
    function column(title, count, body) {
      return '<div class="yard-column"><div class="yard-column-head"><span>' + title + '</span><span class="yard-count">' + count + '</span></div>' + body + '</div>';
    }
    var cards = captured.map(function (o) {
      return '<div class="yard-card"><div class="yard-card-text">' + esc(o.excerpt) + '</div><div class="yard-row-meta">' + esc(o.source) + (o.repo_key ? ' · ' + esc(repoName(o.repo_key)) : '') + ' · ' + esc(shortTime(o.captured_at)) + '</div></div>';
    }).join('') || empty('Nothing waiting.');
    return '<div class="yard-board">' +
      column('Captured', captured.length, cards) +
      column('Candidate', pipeline.candidate || 0, '') +
      column('Active', pipeline.active || 0, '') +
      column('Retired', pipeline.retired || 0, '') + '</div>';
  }

  function personaHTML(files, selected) {
    var names = Object.keys(files || {}).sort();
    if (!names.length) return empty('No persona generated yet.');
    var current = selected && files[selected] != null ? selected : names[0];
    var tree = names.map(function (name) {
      return '<button type="button" class="yard-file' + (name === current ? ' active' : '') + '" data-persona-file="' + esc(name) + '">' + esc(name.replace(/^repositories\/([^/]+)/, function (_, key) { return 'repositories/' + repoName(key); })) + '</button>';
    }).join('');
    return '<div class="yard-persona"><nav class="yard-tree">' + tree + '</nav><pre class="yard-file-view">' + esc(files[current]) + '</pre></div>';
  }

  function activityHTML(events) {
    if (!events || !events.length) return empty('No activity yet.');
    return '<ol class="yard-timeline">' + events.map(function (e) {
      return '<li class="yard-event yard-event--' + esc(e.kind) + '"><span class="yard-event-kind">' + esc(EVENT_LABELS[e.kind] || e.kind) + '</span>' +
        (e.detail ? '<span class="yard-event-detail">' + esc(e.detail) + '</span>' : '') +
        '<span class="yard-row-meta">' + esc(shortTime(e.at)) + (e.rule_id ? ' · ' + esc(e.rule_id.slice(0, 13)) : '') + '</span></li>';
    }).join('') + '</ol>';
  }

  // Recent sessions and the rules each received, with why each was loaded.
  function loadsHTML(sessions) {
    if (!sessions || !sessions.length) return empty('No session has loaded a rule yet.');
    return sessions.map(function (s) {
      return '<div class="yard-subhead">Session ' + esc(s.session.slice(0, 8)) + ' · ' + esc(shortTime(s.updated_at)) + '</div><ul class="yard-list">' + (s.rules || []).map(function (r) {
        return '<li class="yard-row"><div class="yard-row-main"><div class="yard-row-title">' + esc(r.directive || r.id) + '</div><div class="yard-row-meta">' + esc(r.reason || 'reason not recorded') + '</div></div></li>';
      }).join('') + '</ul>';
    }).join('');
  }

  function attentionCount(health) {
    health = health || {};
    var checks = (health.checks || []).filter(function (c) { return c.status === 'fail'; }).length;
    var guards = (health.guards || []).filter(function (g) { return g.status === 'fail'; }).length;
    return checks + guards;
  }

  function healthHTML(health) {
    health = health || {};
    var html = '<ul class="yard-list">' + (health.checks || []).map(function (c) {
      return '<li class="yard-row"><span class="yard-dot yard-dot--' + esc(c.status) + '"></span><div class="yard-row-main"><div class="yard-row-title">' + esc(c.name.replace(/_/g, ' ')) + '</div><div class="yard-row-meta">' + esc(c.detail) + '</div></div></li>';
    }).join('') + '</ul>';
    var guards = health.guards || [];
    if (guards.length) {
      html += '<div class="yard-subhead">Guards</div><ul class="yard-list">' + guards.map(function (g) {
        return '<li class="yard-row"><span class="yard-dot yard-dot--' + (g.status === 'pass' ? 'ok' : 'fail') + '"></span><div class="yard-row-main"><div class="yard-row-title">' + esc(g.kind) + '</div><div class="yard-row-meta">' + esc(g.failure || (g.examples + ' example' + (g.examples === 1 ? '' : 's') + ' pass')) + '</div></div></li>';
      }).join('') + '</ul>';
    }
    return html + '<div class="yard-actions"><button type="button" class="yard-btn yard-btn--primary" data-yard-action="maintenance">Run maintenance</button></div>';
  }

  function reviewHTML(events, since) {
    var recent = (events || []).filter(function (e) { return !since || e.at >= since; });
    var html = '<div class="yard-subhead">Since this review opened</div>' + activityHTML(recent);
    html += '<form class="yard-make-rule"><div class="yard-subhead">Make it a rule</div>' +
      '<input class="yard-input" name="directive" aria-label="Rule" placeholder="Write method definitions over multiple lines." required>' +
      '<div class="yard-make-rule-row"><select class="settings-select" name="scope" aria-label="Scope"><option value="global">Global</option></select>' +
      '<input class="yard-input" name="surfaces" aria-label="Surfaces" placeholder="ruby, naming">' +
      '<button type="submit" class="yard-btn yard-btn--primary">Add rule</button></div></form>';
    return html;
  }

  function defaultFetch(path) {
    return fetch(path).then(function (response) {
      if (!response.ok) throw new Error(path + ' returned ' + response.status);
      return response.json();
    });
  }

  function defaultPost(path, body) {
    return fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body || {}) }).then(function (response) {
      if (!response.ok) return response.text().then(function (text) { throw new Error(text || response.statusText); });
      return response.json();
    });
  }

  // render mounts the Yard into container. opts: { mode: 'tab'|'page',
  // fetchJSON(path), postJSON(path, body), since (ISO time the review opened),
  // onAttention(count) }.
  function render(container, opts) {
    opts = opts || {};
    var mode = opts.mode || 'page';
    var fetchJSON = opts.fetchJSON || defaultFetch;
    var postJSON = opts.postJSON || defaultPost;
    var views = VIEWS.filter(function (v) { return !v.modes || v.modes.indexOf(mode) !== -1; });
    var state = { view: views[0].id, filter: { status: 'active' }, persona: '' };
    container.innerHTML = '<div class="yard yard--' + mode + '"><nav class="yard-nav" aria-label="Preference Yard views">' +
      views.map(function (v) { return '<button type="button" class="yard-nav-item" data-yard-view="' + v.id + '">' + v.label + '</button>'; }).join('') +
      '</nav><div class="yard-body" aria-live="polite"></div></div>';
    var body = container.querySelector('.yard-body');
    // Listeners go on the root this render created, so re-rendering into a
    // persistent pane replaces them instead of stacking duplicates.
    var root = container.querySelector('.yard');

    function show(view) {
      state.view = view;
      Array.prototype.forEach.call(container.querySelectorAll('.yard-nav-item'), function (b) {
        b.classList.toggle('active', b.getAttribute('data-yard-view') === view);
      });
      var load = {
        review: function () { return fetchJSON('/api/yard/activity?limit=200').then(function (e) { return reviewHTML(e, opts.since); }); },
        rules: function () { return fetchJSON('/api/yard/rules').then(function (r) { return filtersHTML() + rulesHTML(r, state.filter); }); },
        pipeline: function () { return fetchJSON('/api/yard/pipeline').then(pipelineHTML); },
        persona: function () { return fetchJSON('/api/yard/persona').then(function (f) { return personaHTML(f, state.persona); }); },
        loads: function () { return fetchJSON('/api/yard/loads').then(loadsHTML); },
        activity: function () { return fetchJSON('/api/yard/activity?limit=200').then(activityHTML); },
        health: function () {
          return fetchJSON('/api/yard/health').then(function (h) {
            if (opts.onAttention) opts.onAttention(attentionCount(h));
            return healthHTML(h);
          });
        },
      }[view];
      return load().then(function (html) { body.innerHTML = html; }).catch(function (err) {
        body.innerHTML = empty('Could not load the Yard: ' + err.message);
      });
    }

    function filtersHTML() {
      var f = state.filter;
      function select(name, options) {
        return '<select class="settings-select" data-yard-filter="' + name + '" aria-label="' + name + '">' + options.map(function (o) {
          return '<option value="' + o[0] + '"' + ((f[name] || '') === o[0] ? ' selected' : '') + '>' + o[1] + '</option>';
        }).join('') + '</select>';
      }
      return '<div class="yard-filters">' +
        select('status', [['active', 'Active'], ['candidate', 'Candidates'], ['superseded', 'Retired'], ['all', 'All']]) +
        select('scope', [['', 'Any scope'], ['global', 'Global'], ['repository', 'Repository']]) + '</div>';
    }

    root.addEventListener('click', function (event) {
      var target = event.target.closest('[data-yard-view],[data-yard-action],[data-persona-file]');
      if (!target) return;
      if (target.hasAttribute('data-yard-view')) { show(target.getAttribute('data-yard-view')); return; }
      if (target.hasAttribute('data-persona-file')) { state.persona = target.getAttribute('data-persona-file'); show('persona'); return; }
      var action = target.getAttribute('data-yard-action');
      var path = action === 'maintenance' ? '/api/yard/maintenance' : '/api/yard/rules/' + encodeURIComponent(target.getAttribute('data-rule-id')) + '/' + action;
      target.disabled = true;
      postJSON(path).then(function () { return show(state.view); }).catch(function (err) {
        target.disabled = false;
        body.insertAdjacentHTML('afterbegin', empty('Could not ' + action + ': ' + err.message));
      });
    });
    root.addEventListener('change', function (event) {
      var name = event.target.getAttribute('data-yard-filter');
      if (!name) return;
      state.filter[name] = event.target.value;
      show('rules');
    });
    root.addEventListener('submit', function (event) {
      if (!event.target.classList.contains('yard-make-rule')) return;
      event.preventDefault();
      var form = event.target;
      var surfaces = form.surfaces.value.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
      postJSON('/api/yard/rules', { directive: form.directive.value, scope: form.scope.value, surfaces: surfaces.length ? surfaces : ['naming'], feedback: form.directive.value })
        .then(function () { return show(state.view); })
        .catch(function (err) { body.insertAdjacentHTML('afterbegin', empty('Could not add the rule: ' + err.message)); });
    });
    // The tab badge counts failing checks, so load Health once up front.
    if (opts.onAttention && state.view !== 'health') {
      fetchJSON('/api/yard/health').then(function (h) { opts.onAttention(attentionCount(h)); }).catch(function () {});
    }
    return show(state.view);
  }

  var api = { render: render, rulesHTML: rulesHTML, filterRules: filterRules, pipelineHTML: pipelineHTML, personaHTML: personaHTML, activityHTML: activityHTML, loadsHTML: loadsHTML, healthHTML: healthHTML, reviewHTML: reviewHTML, attentionCount: attentionCount };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.yard = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
