// hunkyard-settings-panes.js — shared renderers for the Settings overlay.
// Owns ALL three tabs (Settings / Shortcuts / About) so code review and
// live mode mount the same panes without duplication. Mode-specific
// behaviour is injected via the `hooks` and `show` options on
// renderSettingsTab — see below.
//
// Exports on window.hunkyard.settingsPanes:
//   renderShortcutsPane(pane, opts)
//     opts.mode : 'code-review' | 'live' (default: 'code-review')
//                  Filters entries by their `modes` array so live users
//                  don't see code-review-only bindings (j/k, ]/[, c/e/d, …).
//   renderAboutPane(pane, cfg, sessionInfo)
//   renderUpdatesPane(pane, cfg, hooks)
//   renderSettingsTab(pane, opts)
//     opts.mode    : 'code-review' | 'live'
//     opts.cfg     : /api/config response or {}
//     opts.show    : { width, hideResolved, ignoreWhitespace, update,
//                       agent, integration } — booleans, defaulted from
//                       mode. ignoreWhitespace defaults off; the caller enables
//                       it only when code diffs exist (git mode).
//     opts.hooks   : {
//                      applyTheme(choice),                   // required
//                      applyWidth(choice),                   // required if show.width
//                      getHideResolved(), setHideResolved(v),// required if show.hideResolved
//                      onHideResolvedChange(),               // optional, called after toggle
//                      getIgnoreWhitespace(),                // required if show.ignoreWhitespace
//                      setIgnoreWhitespace(v),               // required if show.ignoreWhitespace
//                      onIgnoreWhitespaceChange(),           // optional, called after toggle (reloads diffs)
//                      hasActivePendingUpdates(),             // optional, default false
//                      syncPendingUpdateButtons(),            // optional
//                      announceCopy(),                       // optional
//                    }

(function () {
  'use strict';

  // escapeHTML — delegates to the canonical window.hunkyard.shared.escapeHTML.
  // hunkyard-shared.js loads before this file per index.html script order.
  var escapeHTML = window.hunkyard.shared.escapeHTML;

  function bindingHTML(binding) {
    if (!binding) return '<span class="shortcut-unassigned">Unassigned</span>';
    // En dash is used by the fixed story chapter range and is not a chord.
    var parts = binding.indexOf('–') !== -1 ? [binding] : binding.split('+');
    return parts.map(function (part) { return '<kbd>' + escapeHTML(part) + '</kbd>'; }).join('+');
  }

  function isReservedBinding(binding) {
    var shortcuts = window.hunkyard && window.hunkyard.shortcuts;
    return shortcuts && shortcuts.isReservedBinding ? shortcuts.isReservedBinding(binding) : false;
  }

  // Shortcut groups declare a section; sections render in this order.
  var SHORTCUT_SECTIONS = { general: 'General', review: 'Review', live: 'Live' };

  function renderShortcutsPane(pane, opts) {
    if (!pane) return;
    opts = opts || {};
    var mode = opts.mode || 'code-review';
    var shortcuts = window.hunkyard && window.hunkyard.shortcuts;
    if (!shortcuts) return;
    var html = '';
    html += '<div class="shortcuts-toolbar">';
    html += '<span>Click a shortcut, then press its new keys. Backspace disables it.</span>';
    html += '<button type="button" class="shortcut-reset-all">Reset all</button>';
    html += '</div>';

    var shownSections = {};
    var sectionOrder = Object.keys(SHORTCUT_SECTIONS);
    var orderedGroups = shortcuts.groups.slice().sort(function (a, b) {
      return sectionOrder.indexOf(a.section || 'general') - sectionOrder.indexOf(b.section || 'general');
    });
    orderedGroups.forEach(function (group) {
      var visible = group.shortcuts.filter(function (s) {
        return s.modes && s.modes.indexOf(mode) !== -1;
      });
      if (visible.length === 0) return;
      var section = SHORTCUT_SECTIONS[group.section] || SHORTCUT_SECTIONS.general;
      if (!shownSections[section]) {
        shownSections[section] = true;
        html += '<div class="shortcuts-section-label">' + section + '</div>';
      }
      html += '<div class="shortcuts-group-label">' + group.label + '</div>';
      html += '<table class="shortcuts-table">';
      visible.forEach(function (s) {
        var modeTag = s.mode ? '<span class="shortcut-mode-badge">' + s.mode + '</span>' : '';
        var binding = s.id ? shortcuts.getBinding(s.id) : s.binding;
        var key = bindingHTML(binding);
        if (s.id && !s.fixed) {
          var customized = shortcuts.isCustomized(s.id) ? ' is-customized' : '';
          key = '<button type="button" class="shortcut-binding' + customized + '" data-shortcut-id="' + escapeHTML(s.id)
            + '" data-shortcut-action="' + escapeHTML(s.action)
            + '" aria-pressed="false" aria-label="Change shortcut for ' + escapeHTML(s.action) + '">' + key + '</button>';
        }
        html += '<tr><td>' + key + '</td><td>' + escapeHTML(s.action) + modeTag + '</td></tr>';
      });
      html += '</table>';
    });

    pane.innerHTML = html;

    function rerender() {
      renderShortcutsPane(pane, opts);
      if (typeof opts.onChange === 'function') opts.onChange();
    }

    var reset = pane.querySelector('.shortcut-reset-all');
    if (reset) reset.addEventListener('click', function () {
      shortcuts.resetAll();
      rerender();
    });

    pane.querySelectorAll('.shortcut-binding').forEach(function (button) {
      function restoreBindingLabel() {
        button.classList.remove('is-capturing');
        button.setAttribute('aria-pressed', 'false');
        button.setAttribute('aria-label', 'Change shortcut for ' + (button.dataset.shortcutAction || 'shortcut'));
        button.innerHTML = bindingHTML(shortcuts.getBinding(button.dataset.shortcutId));
      }

      button.addEventListener('click', function () {
        button.classList.add('is-capturing');
        button.setAttribute('aria-pressed', 'true');
        button.setAttribute('aria-label', 'Press new keys for ' + (button.dataset.shortcutAction || 'shortcut'));
        // Keep the same <kbd> box while capturing so single-key rows do not
        // change height when plain button text replaces their binding.
        button.innerHTML = '<kbd>Press keys…</kbd>';
      });
      button.addEventListener('blur', function (e) {
        if (!button.classList.contains('is-capturing')) return;
        var next = e && e.relatedTarget;
        var movesWithinShortcutControls = next && next.closest &&
          next.closest('.shortcut-binding, .shortcut-reset-all');
        if (movesWithinShortcutControls) {
          restoreBindingLabel();
          return;
        }
        rerender();
      });
      button.addEventListener('keydown', function (e) {
        if (!button.classList.contains('is-capturing')) return;
        e.preventDefault();
        e.stopPropagation();
        if (typeof e.stopImmediatePropagation === 'function') e.stopImmediatePropagation();
        if (e.key === 'Escape') { rerender(); return; }

        var binding = (e.key === 'Backspace' || e.key === 'Delete') ? '' : shortcuts.eventToBinding(e);
        if (!binding && e.key !== 'Backspace' && e.key !== 'Delete') return;
        var id = button.dataset.shortcutId;

        function showError(message) {
          restoreBindingLabel();
          var shared = window.hunkyard && window.hunkyard.shared;
          if (shared && shared.showToast) shared.showToast(message, { kind: 'error' });
        }

        if (isReservedBinding(binding)) {
          showError(binding + ' is reserved by Hunkyard.');
          return;
        }
        var conflict = shortcuts.findConflict(id, binding);
        if (conflict) {
          showError(binding + ' is already assigned to “' + conflict.action + '”.');
          return;
        }
        shortcuts.setBinding(id, binding);
        rerender();
      });
    });
  }

  function renderAboutPane(pane, cfg, sessionInfo) {
    if (!pane) return;
    cfg = cfg || {};
    var session = sessionInfo || {};
    var html = '';

    // Version header
    html += '<div class="about-header">';
    html += '<h2>Hunkyard</h2>';
    var ver = cfg.version || 'dev';
    html += '<div class="about-version">' + escapeHTML(ver) + '</div>';
    html += '</div>';

    // Session info
    html += '<div class="settings-section-label">Current Session</div>';
    html += '<div class="about-session"><div class="about-session-grid">';
    var modeLabel = session.vcs_name || session.mode || 'live';
    html += '<span class="about-session-label">Mode</span><span class="about-session-value">' + escapeHTML(modeLabel) + '</span>';
    if (session.mode === 'git' && session.branch) {
      html += '<span class="about-session-label">Branch</span><span class="about-session-value">' + escapeHTML(session.branch) + '</span>';
    }
    if (session.base_ref) {
      html += '<span class="about-session-label">Base</span><span class="about-session-value">' + escapeHTML(session.base_branch_name || session.base_ref) + '</span>';
    }
    if (session.upstream_url) {
      html += '<span class="about-session-label">Upstream</span><span class="about-session-value"><code>' + escapeHTML(session.upstream_url) + '</code></span>';
    }
    html += '<span class="about-session-label">Round</span><span class="about-session-value">' + (session.review_round || 1) + '</span>';
    if (session.files !== undefined) {
      html += '<span class="about-session-label">Files</span><span class="about-session-value">' + (session.files ? session.files.length : 0) + ' changed</span>';
    }
    if (cfg.review_path) {
      html += '<span class="about-session-label">Review file</span><span class="about-session-value"><code>' + escapeHTML(cfg.review_path) + '</code></span>';
    }
    html += '</div></div>';

    // Links
    html += '<div class="settings-section-label">Links</div>';
    html += '<div class="about-links">';
    html += '<a class="about-link" href="https://github.com/BSH2409/hunkyard" target="_blank" rel="noopener"><svg viewBox="0 0 16 16" fill="currentColor"><path d="M8 0c4.42 0 8 3.58 8 8a8.013 8.013 0 0 1-5.45 7.59c-.4.08-.55-.17-.55-.38 0-.27.01-1.13.01-2.2 0-.75-.25-1.23-.54-1.48 1.78-.2 3.65-.88 3.65-3.95 0-.88-.31-1.59-.82-2.15.08-.2.36-1.02-.08-2.12 0 0-.67-.22-2.2.82-.64-.18-1.32-.27-2-.27-.68 0-1.36.09-2 .27-1.53-1.03-2.2-.82-2.2-.82-.44 1.1-.16 1.92-.08 2.12-.51.56-.82 1.28-.82 2.15 0 3.06 1.86 3.75 3.64 3.95-.23.2-.44.55-.51 1.07-.46.21-1.61.55-2.33-.66-.15-.24-.6-.83-1.23-.82-.67.01-.27.38.01.53.34.19.73.9.82 1.13.16.45.68 1.31 2.69.94 0 .67.01 1.3.01 1.49 0 .21-.15.45-.55.38A7.995 7.995 0 0 1 0 8c0-4.42 3.58-8 8-8Z"/></svg>Project source</a>';
    html += '<a class="about-link" href="https://github.com/BSH2409/hunkyard#readme" target="_blank" rel="noopener"><svg viewBox="0 0 16 16" fill="currentColor"><path d="M1 7.775V2.75C1 1.784 1.784 1 2.75 1h5.025c.464 0 .91.184 1.238.513l6.25 6.25a1.75 1.75 0 0 1 0 2.474l-5.026 5.026a1.75 1.75 0 0 1-2.474 0l-6.25-6.25A1.752 1.752 0 0 1 1 7.775Zm1.5 0c0 .066.026.13.073.177l6.25 6.25a.25.25 0 0 0 .354 0l5.025-5.025a.25.25 0 0 0 0-.354l-6.25-6.25a.25.25 0 0 0-.177-.073H2.75a.25.25 0 0 0-.25.25ZM6 5a1 1 0 1 1 0 2 1 1 0 0 1 0-2Z"/></svg>Documentation</a>';
    html += '</div>';

    pane.innerHTML = html;
  }

  // ============================================================
  // Settings tab (Display + Configuration cards). Both modes call this.
  // mode-specific differences are confined to defaults() and hooks; the
  // markup, copy buttons, dismiss buttons, pill indicators are shared.
  // ============================================================

  function defaultsForMode(mode) {
    if (mode === 'live') {
      return {
        width: false,         // width pill is file-mode only
        hideResolved: true,
        ignoreWhitespace: false, // code-diff only; enabled per-call in git mode
        agent: true,
      };
    }
    // code-review default
    return {
      themePreview: true,   // "Preview all themes" link under the theme selects
      width: true,
      hideResolved: true,
      ignoreWhitespace: false, // code-diff only; enabled per-call in git mode
      agent: true,
    };
  }

  function sharedApi() {
    return (window.hunkyard && window.hunkyard.shared) || {};
  }

  // Family names come from local font metadata, so quote them before putting
  // them in a CSS declaration. Custom values still go through sanitizeCodeFont.
  function fontFamilyStack(family) {
    return '"' + String(family).replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/[\r\n\f]/g, ' ') + '", monospace';
  }

  function codeFontOptions(cfg) {
    var options = (sharedApi().CODE_FONT_PRESETS || []).slice();
    var seen = {};
    options.forEach(function (option) { seen[option.stack] = true; });
    (Array.isArray(cfg.code_fonts) ? cfg.code_fonts : []).forEach(function (family, index) {
      if (typeof family !== 'string' || !family.trim()) return;
      var stack = fontFamilyStack(family.trim());
      if (seen[stack]) return;
      seen[stack] = true;
      options.push({ id: 'installed-' + index, label: family.trim(), stack: stack });
    });
    return options;
  }

  function getSetting(key, fallback) {
    var s = window.hunkyard && window.hunkyard.shared;
    return s && s.getSetting ? s.getSetting(key, fallback) : fallback;
  }
  function setSetting(key, value) {
    var s = window.hunkyard && window.hunkyard.shared;
    if (s && s.setSetting) s.setSetting(key, value);
  }

  function updatePillIndicator(pane, indicatorId, values, current) {
    var indicator = pane.querySelector('#' + indicatorId);
    if (!indicator) return;
    var idx = values.indexOf(current);
    if (idx >= 0) {
      indicator.style.left = (idx * (100 / values.length)) + '%';
      indicator.style.width = (100 / values.length) + '%';
    }
  }

  var THEME_ICONS = {
    system: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor"><path fill-rule="evenodd" d="M2 4.25A2.25 2.25 0 0 1 4.25 2h7.5A2.25 2.25 0 0 1 14 4.25v5.5A2.25 2.25 0 0 1 11.75 12h-1.312c.1.128.21.248.328.36a.75.75 0 0 1 .234.545v.345a.75.75 0 0 1-.75.75h-4.5a.75.75 0 0 1-.75-.75v-.345a.75.75 0 0 1 .234-.545c.118-.111.228-.232.328-.36H4.25A2.25 2.25 0 0 1 2 9.75v-5.5Zm2.25-.75a.75.75 0 0 0-.75.75v4.5c0 .414.336.75.75.75h7.5a.75.75 0 0 0 .75-.75v-4.5a.75.75 0 0 0-.75-.75h-7.5Z" clip-rule="evenodd"/></svg>',
    light: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1a.75.75 0 0 1 .75.75v1.5a.75.75 0 0 1-1.5 0v-1.5A.75.75 0 0 1 8 1ZM10.5 8a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0ZM12.95 4.11a.75.75 0 1 0-1.06-1.06l-1.062 1.06a.75.75 0 0 0 1.061 1.062l1.06-1.061ZM15 8a.75.75 0 0 1-.75.75h-1.5a.75.75 0 0 1 0-1.5h1.5A.75.75 0 0 1 15 8ZM11.89 12.95a.75.75 0 0 0 1.06-1.06l-1.06-1.062a.75.75 0 0 0-1.062 1.061l1.061 1.06ZM8 12a.75.75 0 0 1 .75.75v1.5a.75.75 0 0 1-1.5 0v-1.5A.75.75 0 0 1 8 12ZM5.172 11.89a.75.75 0 0 0-1.061-1.062L3.05 11.89a.75.75 0 1 0 1.06 1.06l1.06-1.06ZM4 8a.75.75 0 0 1-.75.75h-1.5a.75.75 0 0 1 0-1.5h1.5A.75.75 0 0 1 4 8ZM4.11 5.172A.75.75 0 0 0 5.173 4.11L4.11 3.05a.75.75 0 1 0-1.06 1.06l1.06 1.06Z"/></svg>',
    dark: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor"><path d="M14.438 10.148c.19-.425-.321-.787-.748-.601A5.5 5.5 0 0 1 6.453 2.31c.186-.427-.176-.938-.6-.748a6.501 6.501 0 1 0 8.585 8.586Z"/></svg>',
  };

  function formatAgentName(slug) {
    return slug.replace(/\b\w/g, function (c) { return c.toUpperCase(); }).replace(/-/g, ' ');
  }

  function wireConfigCardActions(pane, hooks) {
    hooks = hooks || {};

    function markIntegrationMuted(button) {
      var item = button.closest && button.closest('.updates-list-item');
      if (!item) return;
      item.dataset.updateStatus = 'muted';
      var status = item.querySelector('.updates-status');
      if (status) {
        status.className = 'updates-status updates-status--muted';
        status.textContent = 'Muted';
      }
      var list = item.parentNode;
      var firstMissing = list && list.querySelector('[data-update-status="missing"]');
      if (firstMissing) list.insertBefore(item, firstMissing);
    }

    pane.querySelectorAll('[data-dismiss-integration]').forEach(function (integrationDismissBtn) {
      integrationDismissBtn.addEventListener('click', function () {
        var agent = integrationDismissBtn.dataset.dismissIntegration || '';
        var hash = integrationDismissBtn.dataset.hash || '';
        if (!agent || !hash) return;
        var map = getSetting('dismissedIntegrations', {}) || {};
        map[agent] = hash;
        setSetting('dismissedIntegrations', map);
        if (hooks.syncPendingUpdateButtons) hooks.syncPendingUpdateButtons();
        var updateBtn = document.getElementById('updateBtn');
        var pending = hooks.hasActivePendingUpdates ? !!hooks.hasActivePendingUpdates() : false;
        if (updateBtn && !pending) updateBtn.style.display = 'none';
        markIntegrationMuted(integrationDismissBtn);
        integrationDismissBtn.outerHTML = '<span class="config-card-dismissed" id="integrationDismissedNote">Dismissed — will remind you when this integration changes</span>';
      });
    });

    pane.querySelectorAll('[data-dismiss-missing]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var agent = btn.dataset.dismissMissing || '';
        if (!agent) return;
        var map = getSetting('dismissedIntegrations', {}) || {};
        map['missing:' + agent] = true;
        setSetting('dismissedIntegrations', map);
        if (hooks.syncPendingUpdateButtons) hooks.syncPendingUpdateButtons();
        var updateBtn = document.getElementById('updateBtn');
        var pending = hooks.hasActivePendingUpdates ? !!hooks.hasActivePendingUpdates() : false;
        if (updateBtn && !pending) updateBtn.style.display = 'none';
        markIntegrationMuted(btn);
        btn.outerHTML = '<span class="config-card-dismissed">Dismissed — will remind you when this integration changes</span>';
      });
    });

    pane.querySelectorAll('.config-card-copy').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var text = btn.dataset.copy;
        navigator.clipboard.writeText(text).then(function () {
          var iconOnly = btn.classList.contains('updates-command-copy');
          btn.innerHTML = iconOnly ? commandCopyCheckIcon() : '✓ Copied';
          btn.setAttribute('aria-label', 'Copied');
          if (hooks.announceCopy) hooks.announceCopy();
          btn.classList.add('copied');
          setTimeout(function () {
            btn.innerHTML = iconOnly ? commandCopyIcon() : 'Copy';
            btn.setAttribute('aria-label', iconOnly ? 'Copy command' : 'Copy');
            btn.classList.remove('copied');
          }, 1500);
        });
      });
    });
  }

  // ============================================================
  // Updates tab. This is the single home for Hunkyard releases and AI integration
  // maintenance, keeping the main Settings and About tabs focused.
  // ============================================================
  function commandCopyIcon() {
    return (window.hunkyard && window.hunkyard.icons && window.hunkyard.icons.ICON_CLIPBOARD) ||
      '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
  }

  function commandCopyCheckIcon() {
    return (window.hunkyard && window.hunkyard.icons && window.hunkyard.icons.ICON_CHECK_SMALL) ||
      '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6 9 17l-5-5"/></svg>';
  }

  function integrationIconHTML(agent) {
    var asset = agent === 'codex-plugin' ? 'codex' : agent;
    var known = ['claude-code', 'codex'];
    if (known.indexOf(asset) === -1) {
      return '<span class="updates-agent-icon updates-agent-icon--fallback" aria-hidden="true"><svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 3h10v10H3z"/><path d="M6 6h4v4H6z"/></svg></span>';
    }
    var base = 'images/integrations/' + asset;
    return '<span class="updates-agent-icon" aria-hidden="true"><img class="updates-agent-icon-dark" src="' + base + '-dark.svg" alt=""><img class="updates-agent-icon-light" src="' + base + '-light.svg" alt=""></span>';
  }

  function updateCommandHTML(command, label) {
    return '<div class="config-card-cmd updates-command">' +
      (label ? '<span class="config-card-cmd-label">' + escapeHTML(label) + '</span>' : '') +
      '<span>$ ' + escapeHTML(command) + '</span><button type="button" class="config-card-copy updates-command-copy" data-copy="' + escapeHTML(command) + '" aria-label="Copy command">' + commandCopyIcon() + '</button></div>';
  }

  function hunkyardUpdateInstructions(source) {
    if (source === 'go') {
      return '<div class="updates-row-note">Update Hunkyard with Go.</div>' + updateCommandHTML('go install github.com/BSH2409/hunkyard/cmd/hunkyard@latest', '');
    }
    if (source === 'nix') {
      return '<div class="updates-row-note">Update the Nix profile or flake that provides Hunkyard.</div>';
    }
    if (source === 'release') {
      return '<div class="updates-row-note">Download your platform\'s binary from the release page and replace this one.</div>';
    }
    if (source === 'source') {
      return '<div class="updates-row-note">Pull your Hunkyard checkout and rebuild, from the checkout:</div>'
        + updateCommandHTML('git pull && go build -o "$(command -v hunkyard)" ./cmd/hunkyard', '');
    }
    return '<div class="updates-row-note">Download your platform\'s binary from the release page, or pull your checkout and rebuild:</div>'
      + updateCommandHTML('go build -o "$(command -v hunkyard)" ./cmd/hunkyard', '');
  }

  function renderUpdatesPane(pane, cfg, hooks) {
    if (!pane) return;
    cfg = cfg || {};
    hooks = hooks || {};
    var esc = hooks.escape || escapeHTML;
    var html = '';

    html += '<div class="updates-pane">';
    // Hunkyard is local-only and never checks for new releases, so this shows
    // the running version and where to look, not an "up to date" claim.
    html += '<div class="updates-section-head"><span class="settings-section-label">Hunkyard</span></div>';
    var version = cfg.version || 'dev';
    html += '<div class="updates-hunkyard-row"><span class="updates-row-name">Hunkyard</span><span class="updates-version">' + esc(/^\d/.test(version) ? 'v' + version : version) + '</span></div>';
    html += '<div class="updates-hunkyard-detail">' + hunkyardUpdateInstructions(cfg.installation_source)
      + '<a class="updates-release-notes" href="https://github.com/BSH2409/hunkyard/releases" target="_blank" rel="noopener">Release notes and downloads</a></div>';

    if (!cfg.no_integration_check) {
      var dismissedMap = getSetting('dismissedIntegrations', {}) || {};
      var stale = (cfg.stale_integrations || []).map(function (si) { return { item: si, muted: !!si.hash && dismissedMap[si.agent] === si.hash }; });
      var missing = (cfg.missing_integrations || []).map(function (agent) { return { agent: agent, muted: !!dismissedMap['missing:' + agent] }; });
      var currentStale = stale.filter(function (entry) { return !entry.muted; });
      var mutedStale = stale.filter(function (entry) { return entry.muted; });
      var mutedMissing = missing.filter(function (entry) { return entry.muted; });
      var uninstalled = missing.filter(function (entry) { return !entry.muted; });
      var copyAll = [];
      currentStale.concat(mutedStale).forEach(function (entry) {
        if (entry.muted) return;
        (entry.item.hint || '').split('\n').map(function (line) { return line.trim(); }).filter(Boolean).forEach(function (line) {
          var parts = line.split('|');
          copyAll.push((parts.length === 2 ? parts[1] : line).replace(/^Run:\s*/i, ''));
        });
      });
      missing.forEach(function (entry) { if (!entry.muted) copyAll.push('hunkyard install ' + entry.agent); });
      html += '<div class="updates-section-head"><span class="settings-section-label">AI integrations</span>' + (copyAll.length ? '<button type="button" class="updates-copy-all" data-copy-all>Copy all commands</button>' : '') + '</div><div class="updates-list">';
      stale.forEach(function (entry) {
        var si = entry.item;
        var name = formatAgentName(si.agent);
        var commands = [];
        (si.hint || '').split('\n').map(function (line) { return line.trim(); }).filter(Boolean).forEach(function (line) {
          var parts = line.split('|');
          var label = parts.length === 2 ? parts[0] : '';
          var command = (parts.length === 2 ? parts[1] : line).replace(/^Run:\s*/i, '');
          commands.push({ label: label, command: command });
        });
        var status = entry.muted ? 'Muted' : 'Stale';
        html += '<div class="updates-list-item" data-update-status="' + (entry.muted ? 'muted' : 'stale') + '"><button type="button" class="updates-row" data-updates-row aria-expanded="false"><span class="updates-row-icon">' + integrationIconHTML(si.agent) + '</span><span class="updates-row-name">' + esc(name) + '</span><span class="updates-row-preview">' + esc(commands[0] ? commands[0].command : 'Update integration') + '</span><span class="updates-status updates-status--' + (entry.muted ? 'muted' : 'stale') + '">' + status + '</span></button>';
        html += '<div class="updates-row-detail" hidden>';
        commands.forEach(function (item) { html += updateCommandHTML(item.command, item.label); });
        if (si.hash) html += '<div class="config-card-actions config-card-actions--end"><button type="button" class="config-card-dismiss" data-dismiss-integration="' + esc(si.agent) + '" data-hash="' + esc(si.hash) + '">Don\'t remind me until next version</button></div>';
        html += '</div></div>';
      });
      mutedMissing.concat(uninstalled).forEach(function (entry) {
        var agent = entry.agent;
        var name = formatAgentName(agent);
        var installCommand = 'hunkyard install ' + agent;
        var missingStatus = entry.muted ? 'Muted' : 'Not installed';
        html += '<div class="updates-list-item" data-update-status="' + (entry.muted ? 'muted' : 'missing') + '"><button type="button" class="updates-row" data-updates-row aria-expanded="false"><span class="updates-row-icon">' + integrationIconHTML(agent) + '</span><span class="updates-row-name">' + esc(name) + '</span><span class="updates-row-preview">' + esc(installCommand) + '</span><span class="updates-status updates-status--' + (entry.muted ? 'muted' : 'missing') + '">' + missingStatus + '</span></button>';
        html += '<div class="updates-row-detail" hidden><div class="updates-row-note">' + esc(name) + ' is installed on your system but does not have the Hunkyard integration yet.</div>' + updateCommandHTML(installCommand, '') + '<div class="config-card-actions config-card-actions--end"><button type="button" class="config-card-dismiss" data-dismiss-missing="' + esc(agent) + '">Don\'t show again</button></div></div></div>';
      });
      if (!stale.length && !missing.length) html += '<div class="updates-empty">All integrations are up to date.</div>';
      html += '</div>';
    }

    pane.innerHTML = html + '</div>';
    wireConfigCardActions(pane, hooks);
    pane.querySelectorAll('[data-updates-row]').forEach(function (row) {
      row.addEventListener('click', function () {
        var detail = row.nextElementSibling;
        var open = row.getAttribute('aria-expanded') === 'true';
        pane.querySelectorAll('[data-updates-row][aria-expanded="true"]').forEach(function (otherRow) {
          if (otherRow === row) return;
          otherRow.setAttribute('aria-expanded', 'false');
          var otherDetail = otherRow.nextElementSibling;
          if (otherDetail) otherDetail.hidden = true;
        });
        row.setAttribute('aria-expanded', String(!open));
        if (detail) detail.hidden = open;
      });
    });
    var copyAllBtn = pane.querySelector('[data-copy-all]');
    if (copyAllBtn) {
      copyAllBtn.addEventListener('click', function () {
        navigator.clipboard.writeText(copyAll.join('\n')).then(function () {
          copyAllBtn.innerHTML = commandCopyCheckIcon() + '<span>Copied</span>';
          if (hooks.announceCopy) hooks.announceCopy();
          setTimeout(function () { copyAllBtn.innerHTML = commandCopyIcon() + '<span>Copy all commands</span>'; }, 1500);
        });
      });
    }
  }

  function renderSettingsTab(pane, opts) {
    if (!pane) return;
    opts = opts || {};
    var cfg = opts.cfg || {};
    var hooks = opts.hooks || {};
    var defaults = defaultsForMode(opts.mode);
    var show = Object.assign({}, defaults, opts.show || {});
    if (cfg.local_only) { show.agent = false; }
    var esc = (hooks.escape) || escapeHTML;

    var currentTheme = getSetting('theme', 'system');
    var currentWidth = getSetting('width', 'default');

    // Rows go into the General or Review section; review holds the Review
    // rows until the General group closes.
    var html = '';
    var review = '';

    // ---------- General ----------
    html += '<div class="settings-section-label">General</div>';
    html += '<div class="settings-display-group">';

    // Theme row (always shown)
    html += '<div class="settings-display-row">';
    html += '<span class="settings-display-label">Theme</span>';
    html += '<div class="settings-pill settings-pill--theme" id="settingsThemePill" role="group" aria-label="Theme">';
    html += '<div class="settings-pill-indicator" id="settingsThemeIndicator"></div>';
    ['system', 'light', 'dark'].forEach(function (theme) {
      var active = theme === currentTheme ? ' active' : '';
      var label = theme.charAt(0).toUpperCase() + theme.slice(1);
      html += '<button type="button" class="settings-pill-btn' + active +
        '" data-settings-theme="' + theme + '" aria-pressed="' + (theme === currentTheme) +
        '" title="' + label + ' theme">' + THEME_ICONS[theme] + '</button>';
    });
    html += '</div></div>';

    if (opts.mode !== 'live') {
      // --hunkyard-font-mono drives code and diffs in code-review mode. The server
      // only sends installed families which pass its code-monospace check;
      // Custom covers misses.
      var codeFontPresets = codeFontOptions(cfg);
      var currentCodeFont = getSetting('codeFont', '');
      var matchedPreset = null;
      codeFontPresets.forEach(function (p) {
        if (!matchedPreset && p.stack === currentCodeFont) matchedPreset = p;
      });
      var selectedFontId = matchedPreset ? matchedPreset.id : 'custom';
      html += '<div class="settings-display-row">';
      html += '<span class="settings-display-label">Code font</span>';
      html += '<select class="settings-select" id="codeFontSelect" aria-label="Code font">';
      codeFontPresets.forEach(function (p) {
        html += '<option value="' + esc(p.id) + '"' + (p.id === selectedFontId ? ' selected' : '') + '>' + esc(p.label) + '</option>';
      });
      html += '<option value="custom"' + (selectedFontId === 'custom' ? ' selected' : '') + '>Custom…</option>';
      html += '</select>';
      html += '</div>';
      html += '<div class="settings-display-row" id="codeFontCustomRow"' + (selectedFontId === 'custom' ? '' : ' hidden') + '>';
      html += '<label class="settings-display-label settings-display-label--sub" for="codeFontCustomInput">Custom font-family</label>';
      html += '<input type="text" class="settings-text-input" id="codeFontCustomInput" spellcheck="false" autocomplete="off"'
        + ' maxlength="256" placeholder="\'Fira Code\', monospace" value="' + esc(selectedFontId === 'custom' ? currentCodeFont : '') + '">';
      html += '</div>';
    }

    if (hooks.themePalettes && hooks.onRendererSettingChange) {
      var rendererSelects = [
        { key: 'lineNumbers', section: 'review', label: 'Line numbers', fallback: 'on', options: [{ id: 'on', name: 'On' }, { id: 'off', name: 'Off' }] },
        { key: 'lightPalette', label: 'Light theme (UI + code)', fallback: hooks.paletteDefaults.light, options: hooks.themePalettes.filter(function(p) { return p.type === 'light'; }) },
        { key: 'darkPalette', label: 'Dark theme (UI + code)', fallback: hooks.paletteDefaults.dark, options: hooks.themePalettes.filter(function(p) { return p.type === 'dark'; }) },
        { key: 'boostContrast', label: 'Syntax contrast', fallback: 'off', options: [{ id: 'off', name: 'Theme default' }, { id: 'on', name: 'Increased' }] },
        { key: 'codeOverflow', section: 'review', label: 'Long code lines', fallback: 'scroll', options: [{ id: 'scroll', name: 'Horizontal scrolling' }, { id: 'wrap', name: 'Wrap lines' }] },
        { key: 'inlineDiff', section: 'review', label: 'Inline diff highlighting', fallback: 'word-alt', options: [{ id: 'word-alt', name: 'Words (alternate)' }, { id: 'word', name: 'Words' }, { id: 'char', name: 'Characters' }, { id: 'none', name: 'Off' }] },
        { key: 'changeIndicators', section: 'review', label: 'Change indicators', fallback: 'classic', options: [{ id: 'classic', name: '+ / − markers' }, { id: 'bars', name: 'Bars' }, { id: 'none', name: 'None' }] },
        { key: 'unchangedContext', section: 'review', label: 'Unchanged context', fallback: 'collapsed', options: [{ id: 'collapsed', name: 'Collapsed (expandable)' }, { id: 'expanded', name: 'Expand all' }] },
      ];
      // Live/preview mode has no code view: only the theme choice applies.
      if (opts.mode === 'live') {
        rendererSelects = rendererSelects.filter(function(s) { return s.key === 'lightPalette' || s.key === 'darkPalette'; });
      }
      rendererSelects.forEach(function(s) {
        var isPalette = s.key === 'lightPalette' || s.key === 'darkPalette';
        if (isPalette) s.options = s.options.slice().sort(function(a, b) { return a.displayName.localeCompare(b.displayName); });
        // Palettes use the validated saved theme (unknown ids fall back to defaults).
        var value = isPalette ? s.fallback : getSetting(s.key, s.fallback);
        var row = '<div class="settings-display-row"><label class="settings-display-label" for="' + s.key + 'Select">' + esc(s.label) + '</label>';
        row += '<select class="settings-select" id="' + s.key + 'Select" data-renderer-setting="' + s.key + '">';
        s.options.forEach(function(o) {
          row += '<option value="' + esc(o.id) + '"' + (value === o.id ? ' selected' : '') + '>' + esc(o.displayName || o.name || o.id) + '</option>';
        });
        row += '</select></div>';
        if (s.section === 'review') review += row; else html += row;
        if (s.key === 'darkPalette' && show.themePreview) {
          html += '<div class="settings-display-row"><span class="settings-display-label"></span>' +
            '<a class="settings-theme-preview-link" href="/themes" target="_blank" rel="noopener">Preview all themes</a></div>';
        }
      });
    }

    // Width row (file-mode in code review; off in live)
    if (show.width) {
      html += '<div class="settings-display-row">';
      html += '<span class="settings-display-label">Content Width <span style="font-weight:400;color:var(--hunkyard-editor-fg-muted)">(file mode)</span></span>';
      html += '<div class="settings-pill settings-pill--width" id="settingsWidthPill" role="group" aria-label="Content width">';
      html += '<div class="settings-pill-indicator" id="settingsWidthIndicator"></div>';
      ['compact', 'default', 'wide'].forEach(function (w) {
        var active = w === currentWidth ? ' active' : '';
        html += '<button type="button" class="settings-pill-btn' + active +
          '" data-settings-width="' + w + '">' + w.charAt(0).toUpperCase() + w.slice(1) + '</button>';
      });
      html += '</div></div>';
    }

    // Hide resolved row
    if (show.hideResolved && hooks.getHideResolved) {
      var hideResolved = !!hooks.getHideResolved();
      review += '<div class="settings-display-row">';
      review += '<span class="settings-display-label">Hide resolved comments</span>';
      review += '<label class="comments-panel-switch">';
      review += '<input type="checkbox" id="hideResolvedToggle" aria-label="Hide resolved comments"' + (hideResolved ? ' checked' : '') + '>';
      review += '<span class="comments-panel-switch-track"><span class="comments-panel-switch-thumb"></span></span>';
      review += '</label>';
      review += '</div>';
    }

    // Ignore whitespace row (code diffs / git mode only)
    if (show.ignoreWhitespace && hooks.getIgnoreWhitespace) {
      var ignoreWhitespace = !!hooks.getIgnoreWhitespace();
      review += '<div class="settings-display-row">';
      review += '<span class="settings-display-label">Ignore whitespace</span>';
      review += '<label class="comments-panel-switch">';
      review += '<input type="checkbox" id="ignoreWhitespaceToggle" aria-label="Ignore whitespace changes in diffs"' + (ignoreWhitespace ? ' checked' : '') + '>';
      review += '<span class="comments-panel-switch-track"><span class="comments-panel-switch-thumb"></span></span>';
      review += '</label>';
      review += '</div>';
    }

    html += '</div>'; // close the General settings-display-group

    // ---------- Review ----------
    if (review) {
      html += '<div class="settings-section-label">Review</div>';
      html += '<div class="settings-display-group">' + review + '</div>';
    }

    // ---------- Configuration ----------
    if (show.agent) {
      html += '<div class="settings-section-label">Configuration</div>';
      html += '<div class="config-cards">';

      // Agent Command card
      if (cfg.agent_cmd_enabled) {
        html += '<div class="config-card config-card--green"><div class="config-card-header">';
        html += '<span class="config-card-icon" style="color:var(--hunkyard-green)">&#10003;</span>';
        html += '<span class="config-card-title">Agent Command</span>';
        html += '</div>';
        html += '<div class="config-card-cmd-value"><code>' + esc(cfg.agent_cmd || cfg.agent_name || '') + '</code></div>';
        html += '</div>';
      } else {
        html += '<div class="config-card config-card--orange config-card--unconfigured"><div class="config-card-header">';
        html += '<span class="config-card-icon" style="color:var(--hunkyard-yellow)">&#9675;</span>';
        html += '<span class="config-card-title">Agent Command</span>';
        html += '</div>';
        html += '<div class="config-card-body">Edit <code>~/.hunkyard.config.json</code> and set <code>agent_cmd</code> to send comments directly to your AI agent. <a href="https://github.com/BSH2409/hunkyard#send-to-agent-experimental" target="_blank" rel="noopener" style="color:var(--hunkyard-brand)">Learn more</a></div>';
        html += '<div class="config-card-snippet">{"agent_cmd": "claude -p"}\n// Also: "codex exec"</div>';
        html += '</div>';
      }

      html += '</div>'; // close config-cards
    }

    pane.innerHTML = html;

    // ---------- Wire-up ----------
    pane.querySelectorAll('[data-settings-theme]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var t = btn.dataset.settingsTheme;
        if (hooks.applyTheme) hooks.applyTheme(t);
        else { setSetting('theme', t); var s = window.hunkyard && window.hunkyard.shared; if (s && s.applyThemeFromCookie) s.applyThemeFromCookie(); }
        pane.querySelectorAll('[data-settings-theme]').forEach(function (b) {
          var on = b.dataset.settingsTheme === t;
          b.classList.toggle('active', on);
          b.setAttribute('aria-pressed', String(on));
        });
        updatePillIndicator(pane, 'settingsThemeIndicator', ['system', 'light', 'dark'], t);
      });
    });
    updatePillIndicator(pane, 'settingsThemeIndicator', ['system', 'light', 'dark'], currentTheme);

    var fontSelect = pane.querySelector('#codeFontSelect');
    var fontCustomRow = pane.querySelector('#codeFontCustomRow');
    var fontCustomInput = pane.querySelector('#codeFontCustomInput');
    // Applied through the shared helper rather than a hook: no mode needs to do
    // anything extra when the code font changes, unlike theme (mermaid re-init)
    // or width (layout attribute).
    function applyCodeFont(stack) {
      var api = sharedApi();
      return api.setCodeFont ? api.setCodeFont(stack) : '';
    }
    function markInvalidFont(invalid) {
      if (!fontCustomInput) return;
      fontCustomInput.classList.toggle('is-invalid', invalid);
      fontCustomInput.setAttribute('aria-invalid', invalid ? 'true' : 'false');
    }
    if (fontSelect) {
      fontSelect.addEventListener('change', function () {
        var id = fontSelect.value;
        if (id === 'custom') {
          if (fontCustomRow) fontCustomRow.hidden = false;
          if (fontCustomInput) {
            fontCustomInput.focus();
            // An empty custom box means "no override yet" — leave the current
            // font alone until the user types something.
            if (fontCustomInput.value.trim()) applyCodeFont(fontCustomInput.value);
          }
          return;
        }
        if (fontCustomRow) fontCustomRow.hidden = true;
        markInvalidFont(false);
        var preset = codeFontOptions(cfg).filter(function (p) { return p.id === id; })[0];
        applyCodeFont(preset ? preset.stack : '');
      });
    }
    if (fontCustomInput) {
      fontCustomInput.addEventListener('change', function () {
        var raw = fontCustomInput.value;
        var rejected = !!raw.trim() && !applyCodeFont(raw);
        markInvalidFont(rejected);
        if (rejected) {
          var s = sharedApi();
          if (s.showToast) s.showToast('Not a valid font-family value — using the default.', { kind: 'error' });
        }
      });
    }

    if (show.width) {
      pane.querySelectorAll('[data-settings-width]').forEach(function (btn) {
        btn.addEventListener('click', function () {
          var w = btn.dataset.settingsWidth;
          if (hooks.applyWidth) hooks.applyWidth(w);
          pane.querySelectorAll('[data-settings-width]').forEach(function (b) {
            b.classList.toggle('active', b.dataset.settingsWidth === w);
          });
          updatePillIndicator(pane, 'settingsWidthIndicator', ['compact', 'default', 'wide'], w);
        });
      });
      updatePillIndicator(pane, 'settingsWidthIndicator', ['compact', 'default', 'wide'], currentWidth);
    }

    if (show.hideResolved && hooks.getHideResolved) {
      var hrToggle = pane.querySelector('#hideResolvedToggle');
      if (hrToggle) {
        hrToggle.addEventListener('change', function () {
          if (hooks.setHideResolved) hooks.setHideResolved(hrToggle.checked);
          if (hooks.onHideResolvedChange) hooks.onHideResolvedChange();
        });
      }
    }

    if (show.ignoreWhitespace && hooks.getIgnoreWhitespace) {
      var iwToggle = pane.querySelector('#ignoreWhitespaceToggle');
      if (iwToggle) {
        iwToggle.addEventListener('change', function () {
          if (hooks.setIgnoreWhitespace) hooks.setIgnoreWhitespace(iwToggle.checked);
          if (hooks.onIgnoreWhitespaceChange) hooks.onIgnoreWhitespaceChange();
        });
      }
    }

    pane.querySelectorAll('[data-renderer-setting]').forEach(function(select) {
      select.addEventListener('change', function() {
        select.disabled = true;
        Promise.resolve(hooks.onRendererSettingChange(select.dataset.rendererSetting, select.value)).catch(function(err) {
          console.error('Could not update display settings', err);
          var s = sharedApi();
          if (s.showToast) s.showToast('Could not update display settings. Try again.', { kind: 'error' });
        }).finally(function() { select.disabled = false; });
      });
    });
    wireConfigCardActions(pane, hooks);
  }

  window.hunkyard = window.hunkyard || {};
  window.hunkyard.settingsPanes = {
    renderShortcutsPane: renderShortcutsPane,
    renderAboutPane: renderAboutPane,
    renderUpdatesPane: renderUpdatesPane,
    renderSettingsTab: renderSettingsTab,
    fontFamilyStack: fontFamilyStack,
  };
})();
