// hunkyard-yard.js — the Preference Yard: what hunkyard-preferences loads,
// learns and blocks. Rendered into the settings overlay's Preferences tab.
//
// Reads: /api/preferences/{rules,persona,pipeline,loads,activity,health,insights}; writes (same-origin
// POST): /api/preferences/rules, /api/preferences/rules/{id}/{retire,promote}, /api/preferences/maintenance.
// Depends on no other window.hunkyard namespace.
(function () {
  'use strict';

  var VIEWS = [
    { id: 'review', label: 'This review' },
    { id: 'rules', label: 'Rules' },
    { id: 'pipeline', label: 'Pipeline' },
    { id: 'persona', label: 'Persona' },
    { id: 'loads', label: 'Loads' },
    { id: 'activity', label: 'Activity' },
    { id: 'insights', label: 'Insights' },
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

  function errorHTML(message) {
    return '<div class="yard-error" role="alert"><span>' + esc(message) + '</span>' +
      '<button type="button" class="btn btn-sm" data-yard-action="retry">Retry</button></div>';
  }

  function isActive(rule) {
    return rule.status === 'active' || rule.status === 'core';
  }

  var STATUS_LABELS = { active: 'Active', core: 'Core', candidate: 'Candidate', superseded: 'Retired', conflicted: 'Conflicted', retired: 'Retired' };
  var ORIGIN_LABELS = { feedback: 'from feedback', inferred: 'inferred', cli: 'added by hand', imported: 'imported' };

  function statusDot(kind, label) {
    return '<span class="yard-dot yard-dot--' + esc(kind) + '" role="img" aria-label="' + esc(label) + '"></span>';
  }

  function filterRules(rules, filter) {
    filter = filter || {};
    var status = filter.status || 'active';
    var query = String(filter.query || '').trim().toLowerCase();
    return (rules || []).filter(function (rule) {
      if (status === 'active' && !isActive(rule)) return false;
      if (status !== 'active' && status !== 'all' && rule.status !== status) return false;
      if (filter.scope && rule.scope_kind !== filter.scope) return false;
      if (filter.surface && (rule.surfaces || []).indexOf(filter.surface) === -1) return false;
      if (query) {
        var haystack = [rule.directive, (rule.surfaces || []).join(' '), repoName(rule.repo_key), rule.id].join(' ').toLowerCase();
        if (haystack.indexOf(query) === -1) return false;
      }
      return true;
    });
  }

  var RULES_BATCH = 50;

  // rulesHTML renders the first `limit` matching rules. confirmRetire names
  // the rule whose Retire button is waiting for a second click.
  function rulesHTML(rules, filter, limit, confirmRetire) {
    var visible = filterRules(rules, filter);
    if (!visible.length) {
      return empty(filter && filter.query ? 'No rule matches “' + filter.query + '”.' : 'No preferences yet. Corrections and review comments you make become rules here.');
    }
    var max = limit || RULES_BATCH;
    var html = '<ul class="yard-list">';
    visible.slice(0, max).forEach(function (rule) {
      var scope = rule.scope_kind === 'global'
        ? '<span class="yard-label yard-label--global">global</span>'
        : '<span class="yard-label yard-label--repo">' + esc(repoName(rule.repo_key)) + '</span>';
      var surfaces = (rule.surfaces || []).map(function (s) { return '<span class="yard-label">' + esc(s) + '</span>'; }).join('');
      var violations = rule.violations ? '<span class="yard-label yard-label--violated">' + rule.violations + ' violation' + (rule.violations === 1 ? '' : 's') + '</span>' : '';
      var dismissals = rule.dismissals ? '<span class="yard-label" title="Rule findings a reviewer dismissed">' + rule.dismissals + ' dismissed</span>' : '';
      var actions = '';
      if (isActive(rule)) {
        if (rule.scope_kind === 'repository') actions += '<button type="button" class="btn btn-sm yard-btn" data-yard-action="promote" data-rule-id="' + esc(rule.id) + '">Make global</button>';
        actions += confirmRetire === rule.id
          ? '<button type="button" class="btn btn-sm yard-btn yard-btn--danger" data-yard-action="retire" data-rule-id="' + esc(rule.id) + '" data-confirmed="true">Confirm retire</button>'
          : '<button type="button" class="btn btn-sm yard-btn" data-yard-action="retire" data-rule-id="' + esc(rule.id) + '">Retire</button>';
      }
      var observations = rule.observations == null ? '' : rule.observations + ' observation' + (rule.observations === 1 ? '' : 's');
      var meta = [observations, ORIGIN_LABELS[rule.origin] || rule.origin, rule.updated_at ? 'updated ' + shortTime(rule.updated_at) : ''].filter(Boolean).join(' · ');
      html += '<li class="yard-row' + (confirmRetire === rule.id ? ' is-confirming' : '') + '">' + statusDot(rule.status, STATUS_LABELS[rule.status] || rule.status) +
        '<div class="yard-row-main"><div class="yard-row-title">' + esc(rule.directive) + '</div>' +
        '<div class="yard-row-labels">' + scope + surfaces + violations + dismissals + '</div>' +
        (meta ? '<div class="yard-row-meta">' + esc(meta) + '</div>' : '') + '</div>' +
        '<div class="yard-row-actions">' + actions + '</div></li>';
    });
    html += '</ul>';
    var rest = visible.length - max;
    if (rest > 0) {
      html += '<button type="button" class="btn btn-sm yard-more" data-yard-action="more">Show ' + Math.min(rest, RULES_BATCH) + ' more <span class="yard-row-meta">of ' + rest + '</span></button>';
    }
    return html;
  }

  function selectHTML(name, label, options, value) {
    return '<select class="settings-select yard-select" data-yard-filter="' + name + '" aria-label="' + label + '">' + options.map(function (o) {
      return '<option value="' + esc(o[0]) + '"' + ((value || '') === o[0] ? ' selected' : '') + '>' + esc(o[1]) + '</option>';
    }).join('') + '</select>';
  }

  var STATUS_FILTERS = [['active', 'Active'], ['candidate', 'Candidates'], ['superseded', 'Retired'], ['all', 'All']];

  function rulesCountText(rules, filter) {
    var count = filterRules(rules, filter).length;
    var word = { active: 'active', candidate: 'candidate', superseded: 'retired', all: '' }[(filter && filter.status) || 'active'];
    return count + ' ' + (word ? word + ' ' : '') + 'rule' + (count === 1 ? '' : 's');
  }

  // The Rules toolbar: how many rules match, a search box and the filters.
  function rulesToolbarHTML(rules, filter) {
    filter = filter || {};
    var surfaces = {};
    (rules || []).forEach(function (r) { (r.surfaces || []).forEach(function (s) { surfaces[s] = true; }); });
    var surfaceOptions = [['', 'Any surface']].concat(Object.keys(surfaces).sort().map(function (s) { return [s, s]; }));
    return '<span class="yard-tools-count" data-yard-count>' + esc(rulesCountText(rules, filter)) + '</span>' +
      '<input type="search" class="yard-input yard-search" data-yard-search aria-label="Search rules" placeholder="Search rules" value="' + esc(filter.query || '') + '">' +
      selectHTML('status', 'Status', STATUS_FILTERS, filter.status || 'active') +
      selectHTML('scope', 'Scope', [['', 'Any scope'], ['global', 'Global'], ['repository', 'Repository']], filter.scope) +
      selectHTML('surface', 'Surface', surfaceOptions, filter.surface);
  }

  // The Pipeline toolbar carries maintenance, at the right; it explains
  // itself when there is nothing for it to do instead of disappearing.
  function pipelineToolbarHTML(pipeline) {
    pipeline = pipeline || {};
    var captured = (pipeline.captured || []).length;
    var candidates = pipeline.candidate || 0;
    var ready = captured > 0 || candidates > 0;
    var label = ready
      ? 'Consolidate ' + captured + ' captured · ' + candidates + ' candidate' + (candidates === 1 ? '' : 's')
      : 'Nothing waiting for maintenance';
    return '<span class="yard-tools-status" data-yard-tools-status role="status"></span>' +
      '<div class="yard-tools-end"><button type="button" class="btn btn-sm btn-primary yard-maintenance" data-yard-action="maintenance"' +
      (ready ? '' : ' disabled title="Nothing is captured or waiting as a candidate"') + '>' + esc(label) + '</button></div>';
  }

  // The board's columns follow the server's grouping: candidate; active and
  // core; everything else (superseded, conflicted, retired) is retired.
  function pipelineStage(status) {
    if (status === 'candidate') return 'candidate';
    if (status === 'active' || status === 'core') return 'active';
    return 'retired';
  }

  var PIPELINE_CARD_LIMIT = 50;

  function ruleCardsHTML(rules) {
    var shown = rules.slice(0, PIPELINE_CARD_LIMIT).map(function (r) {
      var where = r.scope_kind === 'repository' ? repoName(r.repo_key || '') : 'global';
      return '<div class="yard-card"><div class="yard-card-text yard-clamp" title="' + esc(r.directive) + '">' + esc(r.directive) + '</div>' +
        '<div class="yard-row-meta">' + esc(where) + ((r.surfaces || []).length ? ' · ' + esc(r.surfaces.join(', ')) : '') + '</div></div>';
    }).join('');
    var more = rules.length - PIPELINE_CARD_LIMIT;
    return (shown || '<div class="yard-column-empty">None</div>') + (more > 0 ? '<div class="yard-row-meta yard-column-more">' + more + ' more in Rules</div>' : '');
  }

  function pipelineHTML(pipeline, rules) {
    pipeline = pipeline || {};
    var captured = pipeline.captured || [];
    var byStage = { candidate: [], active: [], retired: [] };
    (rules || []).forEach(function (r) { byStage[pipelineStage(r.status)].push(r); });
    function column(title, count, body) {
      return '<section class="yard-column" aria-label="' + title + '"><div class="yard-column-head"><span>' + title + '</span><span class="yard-count">' + count + '</span></div>' + body + '</section>';
    }
    var cards = captured.map(function (o) {
      return '<div class="yard-card"><div class="yard-card-text yard-clamp" title="' + esc(o.excerpt) + '">' + esc(o.excerpt) + '</div><div class="yard-row-meta">' + esc(o.source) + (o.repo_key ? ' · ' + esc(repoName(o.repo_key)) : '') + ' · ' + esc(shortTime(o.captured_at)) + '</div></div>';
    }).join('') || '<div class="yard-column-empty">Nothing waiting</div>';
    return '<div class="yard-board">' +
      column('Captured', captured.length, cards) +
      column('Candidate', pipeline.candidate || 0, ruleCardsHTML(byStage.candidate)) +
      column('Active', pipeline.active || 0, ruleCardsHTML(byStage.active)) +
      column('Retired', pipeline.retired || 0, ruleCardsHTML(byStage.retired)) + '</div>';
  }

  // Persona files are generated markdown; the leading HTML comments are the
  // generated-file header, not content.
  function stripGeneratedHeader(markdown) {
    return String(markdown || '').replace(/^(\s*<!--[\s\S]*?-->\s*)+/, '');
  }

  // markdownRenderer returns a sanitizing markdown renderer when the page has
  // markdown-it and DOMPurify, otherwise null (the file shows as text).
  function markdownRenderer() {
    if (typeof window === 'undefined' || typeof window.markdownit !== 'function' || !window.DOMPurify) return null;
    var md = window.markdownit({ html: false, linkify: true });
    return function (text) { return window.DOMPurify.sanitize(md.render(text)); };
  }

  function personaHTML(files, selected, renderMarkdown) {
    var names = Object.keys(files || {}).sort();
    if (!names.length) return empty('No persona generated yet.');
    var current = selected && files[selected] != null ? selected : names[0];
    var tree = names.map(function (name) {
      return '<button type="button" class="yard-file' + (name === current ? ' active' : '') + '" data-persona-file="' + esc(name) + '"' + (name === current ? ' aria-current="true"' : '') + '>' +
        esc(name.replace(/^repositories\/([^/]+)/, function (_, key) { return 'repositories/' + repoName(key); })) + '</button>';
    }).join('');
    var view = renderMarkdown
      ? '<article class="yard-file-view yard-markdown">' + renderMarkdown(stripGeneratedHeader(files[current])) + '</article>'
      : '<pre class="yard-file-view">' + esc(files[current]) + '</pre>';
    return '<div class="yard-persona"><nav class="yard-tree" aria-label="Persona files">' + tree + '</nav>' + view + '</div>';
  }

  function activityHTML(events) {
    if (!events || !events.length) return empty('No activity yet.');
    return '<ol class="yard-timeline">' + events.map(function (e) {
      return '<li class="yard-event yard-event--' + esc(e.kind) + '"><span class="yard-event-kind">' + esc(EVENT_LABELS[e.kind] || e.kind) + '</span>' +
        (e.detail ? '<span class="yard-event-detail">' + esc(e.detail) + '</span>' : '') +
        '<span class="yard-row-meta">' + esc(shortTime(e.at)) + (e.rule_id ? ' · <span title="' + esc(e.rule_id) + '">' + esc(e.rule_id.slice(0, 13)) + '</span>' : '') + '</span></li>';
    }).join('') + '</ol>';
  }

  var INSIGHT_WINDOWS = [[7, '7 days'], [30, '30 days'], [90, '90 days'], [0, 'All time']];

  function duration(seconds) {
    var minutes = Math.round((seconds || 0) / 60);
    if (minutes < 60) return minutes + 'm';
    var hours = Math.floor(minutes / 60);
    return hours + 'h' + (minutes % 60 ? ' ' + (minutes % 60) + 'm' : '');
  }

  function insightsToolbarHTML(days) {
    return '<div class="yard-filter" role="group" aria-label="Window">' + INSIGHT_WINDOWS.map(function (w) {
      var on = w[0] === days;
      return '<button type="button" class="toggle-btn' + (on ? ' active' : '') + '" data-yard-days="' + w[0] + '" aria-pressed="' + on + '">' + w[1] + '</button>';
    }).join('') + '</div>';
  }

  // How the reviews of the window went, from hunkyard stats.
  function insightsHTML(ri) {
    var window = ri.window_days ? 'the last ' + ri.window_days + ' days' : 'all time';
    if (!ri.reviews) return empty('No reviews ended in ' + window + '.');
    var stat = function (value, label) { return '<div class="yard-stat"><strong>' + esc(value) + '</strong> ' + esc(label) + '</div>'; };
    var round1 = function (n) { return Math.round((n || 0) * 10) / 10; };
    var stats = '<div class="yard-stats">' +
      stat(ri.reviews, ri.reviews === 1 ? 'review' : 'reviews') + stat(ri.approved, 'approved') +
      stat(round1(ri.rounds_per_review.median), 'median rounds (max ' + round1(ri.rounds_per_review.max) + ')') +
      stat(round1(ri.comments_per_round), 'comments per round') +
      (ri.approved ? stat(duration(ri.approve_seconds.median), 'median time to approve') : '') + '</div>';
    var labels = Object.keys(ri.by_label || {}).sort().map(function (k) {
      return '<span class="yard-label">' + esc(k) + ' ' + ri.by_label[k] + '</span>';
    }).join('');
    var r = ri.by_resolution || {};
    var comments = labels ? '<h4 class="yard-subhead">Comments</h4><div class="yard-row-labels">' + labels + '</div>' +
      '<p class="yard-row-meta">' + (r.resolved || 0) + ' resolved · ' + (r.unresolved || 0) + ' open · ' + (r.claim_unchanged || 0) + ' claimed fixed but unchanged</p>' : '';
    var repos = (ri.by_repository || []).length ? '<h4 class="yard-subhead">Repositories</h4><ul class="yard-list">' + ri.by_repository.map(function (x) {
      return '<li class="yard-row"><span class="yard-row-title">' + esc(x.repository) + '</span><span class="yard-row-meta">' + x.reviews + ' review' + (x.reviews === 1 ? '' : 's') + ' · ' + x.comments + ' comments</span></li>';
    }).join('') + '</ul>' : '';
    var rules = (ri.rules || []).length ? '<h4 class="yard-subhead">Rules behind comments</h4><ul class="yard-list">' + ri.rules.map(function (x) {
      return '<li class="yard-row"><span class="yard-row-title">' + esc(x.directive || x.rule_id) + '</span><span class="yard-row-meta">' +
        x.comments + ' comments · ' + x.confirmed + ' confirmed · ' + x.dismissed + ' dismissed</span></li>';
    }).join('') + '</ul>' : '';
    return '<div class="yard-insights">' + stats + comments + repos + rules + '</div>';
  }

  // Recent sessions and the rules each received, with why each was loaded.
  function loadsHTML(sessions) {
    if (!sessions || !sessions.length) return empty('No session has loaded a rule yet.');
    return sessions.map(function (s) {
      return '<div class="yard-subhead">Session <code>' + esc(String(s.session).slice(0, 8).toLowerCase()) + '</code> · ' + esc(shortTime(s.updated_at)) + '</div><ul class="yard-list">' + (s.rules || []).map(function (r) {
        return '<li class="yard-row"><div class="yard-row-main"><div class="yard-row-title">' + esc(r.directive || r.id) + '</div><div class="yard-row-meta">' + esc(r.reason || 'reason not recorded') + '</div></div></li>';
      }).join('') + '</ul>';
    }).join('');
  }

  // The tab badge counts what needs the user: failing or warning checks and
  // failing guards.
  function attentionCount(health) {
    health = health || {};
    var checks = (health.checks || []).filter(function (c) { return c.status === 'fail' || c.status === 'warn'; }).length;
    var guards = (health.guards || []).filter(function (g) { return g.status === 'fail'; }).length;
    return checks + guards;
  }

  var CHECK_LABELS = { ok: 'Passing', warn: 'Warning', fail: 'Failing' };

  var CHECK_ORDER = { fail: 0, warn: 1 };

  function healthHTML(health) {
    health = health || {};
    var attention = attentionCount(health);
    var checks = (health.checks || []).slice().sort(function (a, b) {
      return (a.status in CHECK_ORDER ? CHECK_ORDER[a.status] : 2) - (b.status in CHECK_ORDER ? CHECK_ORDER[b.status] : 2);
    });
    var html = '<p class="yard-health-summary' + (attention ? ' needs-attention' : '') + '">' + (attention ? attention + ' need attention' : 'Everything is healthy') + '</p>';
    html += '<ul class="yard-list">' + checks.map(function (c) {
      return '<li class="yard-row">' + statusDot(c.status, CHECK_LABELS[c.status] || c.status) + '<div class="yard-row-main"><div class="yard-row-title">' + esc(String(c.name).replace(/_/g, ' ')) + '</div><div class="yard-row-meta">' + esc(c.detail) + '</div></div></li>';
    }).join('') + '</ul>';
    var guards = health.guards || [];
    if (guards.length) {
      html += '<div class="yard-subhead">Guards</div><ul class="yard-list">' + guards.map(function (g) {
        var dot;
        var detail;
        if (g.status !== 'pass') {
          dot = statusDot('fail', 'Failing');
          detail = g.failure || 'failing';
        } else if (!g.examples) {
          dot = statusDot('none', 'No examples');
          detail = 'No examples yet: add --match and --not-match examples to prove what it blocks';
        } else {
          dot = statusDot('ok', 'Passing');
          detail = g.examples + ' example' + (g.examples === 1 ? '' : 's') + ' pass';
        }
        return '<li class="yard-row">' + dot + '<div class="yard-row-main"><div class="yard-row-title">' + esc(g.kind) + '</div><div class="yard-row-meta">' + esc(detail) + '</div></div></li>';
      }).join('') + '</ul>';
    }
    return html;
  }

  function reviewHTML(events, since) {
    var recent = (events || []).filter(function (e) { return !since || e.at >= since; });
    var html = '<div class="yard-subhead">Since this review opened</div>' + (recent.length ? activityHTML(recent) : empty('Nothing learned from this review yet.'));
    html += '<form class="yard-make-rule"><div class="yard-subhead">Make it a rule</div>' +
      '<input class="yard-input" name="directive" aria-label="Rule" placeholder="Write method definitions over multiple lines." required>' +
      '<div class="yard-make-rule-row"><select class="settings-select yard-select" name="scope" aria-label="Scope"><option value="global">Every repository</option><option value="repository">This repository</option></select>' +
      '<input class="yard-input" name="surfaces" aria-label="Surfaces" placeholder="Surfaces, e.g. ruby, naming" required>' +
      '<button type="submit" class="btn btn-sm btn-primary">Add rule</button></div></form>';
    return html;
  }

  function attentionBadge(count) {
    return count ? '<span class="yard-tab-badge" aria-label="' + count + ' need attention">' + count + '</span>' : '';
  }

  // tabsHTML renders the view tabs; Health carries the attention count the
  // Preferences tab badge shows.
  function tabsHTML(active, attention) {
    return '<div class="yard-tabs" role="tablist" aria-label="Preferences views">' + VIEWS.map(function (v) {
      var on = v.id === active;
      return '<button type="button" role="tab" class="yard-nav-item' + (on ? ' active' : '') + '" id="yard-tab-' + v.id + '" data-yard-view="' + v.id + '" aria-controls="yard-panel" aria-selected="' + on + '" tabindex="' + (on ? '0' : '-1') + '">' + v.label + (v.id === 'health' ? attentionBadge(attention) : '') + '</button>';
    }).join('') + '</div>';
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

  function setAttr(el, name, value) {
    if (el && el.setAttribute) el.setAttribute(name, value);
  }

  function viewLabel(id) {
    for (var i = 0; i < VIEWS.length; i++) if (VIEWS[i].id === id) return VIEWS[i].label;
    return id;
  }

  // render mounts the Yard into container. opts: { fetchJSON(path),
  // postJSON(path, body), since (ISO time the review opened),
  // onAttention(count) }.
  function render(container, opts) {
    opts = opts || {};
    var fetchJSON = opts.fetchJSON || defaultFetch;
    var postJSON = opts.postJSON || defaultPost;
    var state = { view: VIEWS[0].id, filter: { status: 'active' }, persona: '', limit: RULES_BATCH, confirmRetire: '', rules: [], insightsDays: 30 };
    // Responses are kept until something changes them, so switching back to
    // a view is instant; every write clears them.
    var cache = {};
    function load(path) {
      if (!cache[path]) {
        cache[path] = fetchJSON(path).catch(function (err) {
          delete cache[path];
          throw err;
        });
      }
      return cache[path];
    }
    function invalidate() { cache = {}; }

    container.innerHTML = '<div class="yard"><div class="yard-bar">' + tabsHTML(state.view) +
      '<div class="yard-tools" data-yard-tools></div></div>' +
      '<div class="yard-status yard-visually-hidden" role="status" aria-live="polite"></div>' +
      '<div class="yard-body" id="yard-panel" role="tabpanel" tabindex="-1" aria-labelledby="yard-tab-' + state.view + '"></div></div>';
    // Listeners go on the root this render created, so re-rendering into a
    // persistent pane replaces them instead of stacking duplicates.
    var root = container.querySelector('.yard');
    var body = container.querySelector('.yard-body');
    var tools = container.querySelector('.yard-tools');
    var announcer = container.querySelector('.yard-status');
    var scroller = container.closest ? container.closest('.settings-content') : null;

    function announce(text) { if (announcer) announcer.textContent = text; }

    function selectTab(view) {
      Array.prototype.forEach.call(container.querySelectorAll('[role="tab"]'), function (tab) {
        var on = tab.getAttribute('data-yard-view') === view;
        if (tab.classList) tab.classList.toggle('active', on);
        tab.setAttribute('aria-selected', String(on));
        tab.setAttribute('tabindex', on ? '0' : '-1');
      });
      setAttr(body, 'aria-labelledby', 'yard-tab-' + view);
    }

    function setBusy(on) {
      if (on) setAttr(body, 'aria-busy', 'true');
      else if (body && body.removeAttribute) body.removeAttribute('aria-busy');
      setAttr(root, 'data-busy', on ? 'true' : 'false');
    }

    // The Persona file list sticks just under the pinned bar, whose height
    // depends on the view's tools.
    function syncBarHeight() {
      var bar = container.querySelector('.yard-bar');
      var tree = body.querySelector && body.querySelector('.yard-tree');
      if (tree && tree.style && bar && bar.offsetHeight) tree.style.top = (bar.offsetHeight - 6) + 'px';
    }

    var loaders = {
      review: function () { return load('/api/preferences/activity?limit=200').then(function (e) { return { body: reviewHTML(e, opts.since) }; }); },
      rules: function () {
        return load('/api/preferences/rules').then(function (r) {
          state.rules = r || [];
          return { tools: rulesToolbarHTML(state.rules, state.filter), body: rulesHTML(state.rules, state.filter, state.limit, state.confirmRetire) };
        });
      },
      pipeline: function () {
        return Promise.all([load('/api/preferences/pipeline'), load('/api/preferences/rules')]).then(function (r) {
          return { tools: pipelineToolbarHTML(r[0]), body: pipelineHTML(r[0], r[1]) };
        });
      },
      persona: function () { return load('/api/preferences/persona').then(function (f) { return { body: personaHTML(f, state.persona, markdownRenderer()) }; }); },
      loads: function () { return load('/api/preferences/loads').then(function (l) { return { body: loadsHTML(l) }; }); },
      activity: function () { return load('/api/preferences/activity?limit=200').then(function (e) { return { body: activityHTML(e) }; }); },
      insights: function () {
        return load('/api/preferences/insights?days=' + state.insightsDays).then(function (ri) {
          return { tools: insightsToolbarHTML(state.insightsDays), body: insightsHTML(ri) };
        });
      },
      health: function () {
        return load('/api/preferences/health').then(function (h) {
          noteAttention(attentionCount(h));
          return { body: healthHTML(h) };
        });
      },
    };

    var loadSeq = 0;
    // show switches to view (or reloads it in place) and resolves once its
    // content is on screen. The tab is selected at once; until the content
    // arrives the previous content stays dimmed and inert, never presented
    // as the new view's.
    function show(view) {
      var switching = view !== state.view;
      state.view = view;
      var seq = ++loadSeq;
      selectTab(view);
      if (switching) {
        state.limit = RULES_BATCH;
        state.confirmRetire = '';
        if (tools) tools.innerHTML = '';
        if (scroller) scroller.scrollTop = 0;
      }
      setBusy(true);
      return loaders[view]().then(function (out) {
        if (seq !== loadSeq) return;
        if (tools) tools.innerHTML = out.tools || '';
        body.innerHTML = out.body;
        setBusy(false);
        announce(viewLabel(view) + ' loaded');
        syncBarHeight();
      }).catch(function (err) {
        if (seq !== loadSeq) return;
        if (tools) tools.innerHTML = '';
        body.innerHTML = errorHTML('Could not load ' + viewLabel(view) + ': ' + err.message);
        setBusy(false);
      });
    }

    // Picking a persona file swaps only the file's content, so the file list
    // keeps its scroll position and the picked file keeps focus.
    function showPersonaFile(button) {
      state.persona = button.getAttribute('data-persona-file');
      load('/api/preferences/persona').then(function (files) {
        var html = personaHTML(files, state.persona, markdownRenderer());
        var holder = document.createElement('div');
        holder.innerHTML = html;
        var next = holder.querySelector('.yard-file-view');
        var current = body.querySelector('.yard-file-view');
        if (!next || !current) { body.innerHTML = html; return; }
        current.replaceWith(next);
        Array.prototype.forEach.call(body.querySelectorAll('[data-persona-file]'), function (b) {
          var on = b.getAttribute('data-persona-file') === state.persona;
          b.classList.toggle('active', on);
          if (on) b.setAttribute('aria-current', 'true');
          else b.removeAttribute('aria-current');
        });
        button.focus();
      }).catch(function (err) { showError('Could not load ' + state.persona + ': ' + err.message); });
    }

    // Filtering, searching and paging redraw the list from the loaded rules,
    // leaving the toolbar (and the search box's focus) alone.
    function redrawRules() {
      body.innerHTML = rulesHTML(state.rules, state.filter, state.limit, state.confirmRetire);
      var count = container.querySelector('[data-yard-count]');
      if (count) count.textContent = rulesCountText(state.rules, state.filter);
    }

    function showError(message) {
      var old = body.querySelector && body.querySelector('.yard-error');
      if (old && old.remove) old.remove();
      if (body.insertAdjacentHTML) body.insertAdjacentHTML('afterbegin', errorHTML(message));
    }

    var VERBS = { retire: 'retire the rule', promote: 'make the rule global', maintenance: 'start maintenance' };

    function act(target, action) {
      var path = action === 'maintenance' ? '/api/preferences/maintenance' : '/api/preferences/rules/' + encodeURIComponent(target.getAttribute('data-rule-id')) + '/' + action;
      target.disabled = true;
      setAttr(target, 'aria-busy', 'true');
      if (action === 'maintenance') target.textContent = 'Running…';
      return postJSON(path).then(function () {
        invalidate();
        state.confirmRetire = '';
        return show(state.view).then(function () {
          if (action === 'maintenance') {
            var note = container.querySelector('[data-yard-tools-status]');
            if (note) note.textContent = 'Maintenance started. Its result appears in Activity.';
          }
          announce({ retire: 'Rule retired', promote: 'Rule made global', maintenance: 'Maintenance started' }[action]);
          if (action !== 'maintenance' && body.focus) body.focus();
        });
      }).catch(function (err) {
        target.disabled = false;
        if (target.removeAttribute) target.removeAttribute('aria-busy');
        showError('Could not ' + VERBS[action] + ': ' + err.message);
      });
    }

    root.addEventListener('click', function (event) {
      var target = event.target.closest('[data-yard-view],[data-yard-action],[data-persona-file],[data-yard-days]');
      if (!target) return;
      if (target.hasAttribute('data-yard-days')) { state.insightsDays = Number(target.getAttribute('data-yard-days')); show('insights'); return; }
      if (target.hasAttribute('data-yard-view')) { show(target.getAttribute('data-yard-view')); return; }
      if (target.hasAttribute('data-persona-file')) { showPersonaFile(target); return; }
      var action = target.getAttribute('data-yard-action');
      if (action === 'retry') { invalidate(); show(state.view); return; }
      if (action === 'more') { state.limit += RULES_BATCH; redrawRules(); return; }
      if (action === 'retire' && !target.hasAttribute('data-confirmed')) {
        state.confirmRetire = target.getAttribute('data-rule-id');
        redrawRules();
        var confirm = body.querySelector('[data-confirmed]');
        if (confirm) confirm.focus();
        return;
      }
      act(target, action);
    });

    root.addEventListener('keydown', function (event) {
      if (event.key === 'Escape' && state.confirmRetire) {
        event.stopPropagation();
        state.confirmRetire = '';
        redrawRules();
        return;
      }
      var tab = event.target.closest && event.target.closest('[role="tab"]');
      if (!tab) return;
      var index = -1;
      for (var i = 0; i < VIEWS.length; i++) if (VIEWS[i].id === tab.getAttribute('data-yard-view')) index = i;
      var next = { ArrowRight: index + 1, ArrowLeft: index - 1, Home: 0, End: VIEWS.length - 1 }[event.key];
      if (next === undefined) return;
      event.preventDefault();
      event.stopPropagation();
      var id = VIEWS[(next + VIEWS.length) % VIEWS.length].id;
      show(id);
      var nextTab = container.querySelector('#yard-tab-' + id);
      if (nextTab) nextTab.focus();
    });

    root.addEventListener('input', function (event) {
      if (!event.target.hasAttribute || !event.target.hasAttribute('data-yard-search')) return;
      state.filter.query = event.target.value;
      state.limit = RULES_BATCH;
      redrawRules();
    });

    root.addEventListener('change', function (event) {
      var name = event.target.getAttribute && event.target.getAttribute('data-yard-filter');
      if (!name) return;
      state.filter[name] = event.target.value;
      state.limit = RULES_BATCH;
      redrawRules();
    });

    root.addEventListener('submit', function (event) {
      if (!event.target.classList.contains('yard-make-rule')) return;
      event.preventDefault();
      var form = event.target;
      var surfaces = form.surfaces.value.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
      if (!surfaces.length) { showError('Name at least one surface, for example ruby or naming.'); return; }
      postJSON('/api/preferences/rules', { directive: form.directive.value, scope: form.scope.value, surfaces: surfaces, feedback: form.directive.value })
        .then(function () {
          invalidate();
          return show(state.view).then(function () { announce('Rule added'); });
        })
        .catch(function (err) { showError('Could not add the rule: ' + err.message); });
    });

    // The Preferences tab badge counts what needs attention in Health, so
    // Health is loaded up front and its own tab shows the same number.
    function noteAttention(count) {
      var tab = container.querySelector('#yard-tab-health');
      if (tab) {
        var old = tab.querySelector && tab.querySelector('.yard-tab-badge');
        if (old && old.remove) old.remove();
        if (count && tab.insertAdjacentHTML) tab.insertAdjacentHTML('beforeend', attentionBadge(count));
      }
      if (opts.onAttention) opts.onAttention(count);
    }
    load('/api/preferences/health').then(function (h) { noteAttention(attentionCount(h)); }).catch(function () {});
    return show(state.view);
  }

  var api = {
    render: render, tabsHTML: tabsHTML, rulesHTML: rulesHTML, rulesToolbarHTML: rulesToolbarHTML, filterRules: filterRules,
    pipelineHTML: pipelineHTML, pipelineToolbarHTML: pipelineToolbarHTML, personaHTML: personaHTML, activityHTML: activityHTML,
    loadsHTML: loadsHTML, healthHTML: healthHTML, insightsHTML: insightsHTML, insightsToolbarHTML: insightsToolbarHTML, reviewHTML: reviewHTML, attentionCount: attentionCount, errorHTML: errorHTML,
  };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.yard = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
