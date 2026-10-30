// hunkyard-comment-templates.js — template bar and saved-snippet CRUD.
// Vanilla JS, no module loader. Exports onto window.hunkyard.commentTemplates.
//
// Depends on: window.hunkyard.shared.getCookie, window.hunkyard.shared.setCookie
// Cookie: `hunkyard-templates` (separate from hunkyard-settings — user-defined, can be longer).
// Each template is `{ title, body }`; the chip shows the title (falling back to
// the body when untitled). Older cookies stored bare body strings, which read as
// untitled templates.

(function () {
  'use strict';

  var ns = (window.hunkyard = window.hunkyard || {});
  var shared = ns.shared;

  // --- CRUD ---

  function normalizeTemplate(entry) {
    if (typeof entry === 'string') return { title: '', body: entry };
    if (entry && typeof entry.body === 'string') {
      return { title: typeof entry.title === 'string' ? entry.title : '', body: entry.body };
    }
    return null;
  }

  function getTemplates() {
    try {
      var raw = shared.getCookie('hunkyard-templates');
      if (raw) {
        var parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) return parsed.map(normalizeTemplate).filter(Boolean);
      }
    } catch (_) {}
    return [];
  }

  function saveTemplates(templates) {
    shared.setCookie('hunkyard-templates', JSON.stringify(templates));
  }

  // --- DOM ---

  /**
   * Build the template bar element.
   * @param {Object} opts
   * @param {function(string): void} opts.onInsert — called with the template body when user picks one
   * @param {function({title: string, body: string}): void} opts.onSaveNew — called after a new template is saved
   * @returns {HTMLElement} the bar element (caller inserts into DOM)
   */
  function buildTemplateBar(opts) {
    var onInsert = opts.onInsert;
    var onSaveNew = opts.onSaveNew;

    var bar = document.createElement('div');
    bar.className = 'comment-template-bar';

    function populate() {
      bar.innerHTML = '';
      var templates = getTemplates();
      if (templates.length === 0) {
        bar.style.display = 'none';
        return;
      }
      bar.style.display = '';
      templates.forEach(function (tmpl, i) {
        var chip = document.createElement('button');
        chip.className = 'template-chip';
        chip.title = tmpl.body;

        var label = document.createElement('span');
        label.className = 'template-chip-label';
        label.textContent = tmpl.title || tmpl.body;
        chip.appendChild(label);

        var del = document.createElement('span');
        del.className = 'template-chip-delete';
        del.textContent = '×';
        del.title = 'Remove template';
        del.addEventListener('click', function (e) {
          e.preventDefault();
          e.stopPropagation();
          var t = getTemplates();
          t.splice(i, 1);
          saveTemplates(t);
          populate();
        });
        chip.appendChild(del);

        chip.addEventListener('click', function (e) {
          e.preventDefault();
          if (onInsert) onInsert(tmpl.body);
        });

        bar.appendChild(chip);
      });
    }

    populate();

    // Expose a refresh handle so callers can re-populate after saving.
    bar._populate = populate;

    // Wire up the onSaveNew callback so external "save template" buttons can
    // call bar._saveNew(title, body) and the bar refreshes automatically.
    bar._saveNew = function (title, body) {
      if (!body) return;
      var tmpl = { title: title || '', body: body };
      var t = getTemplates();
      t.push(tmpl);
      saveTemplates(t);
      populate();
      if (onSaveNew) onSaveNew(tmpl);
    };

    return bar;
  }

  // --- Public API ---

  var api = {
    getTemplates: getTemplates,
    saveTemplates: saveTemplates,
    buildTemplateBar: buildTemplateBar,
  };

  ns.commentTemplates = api;

  // CommonJS dual-export for unit tests.
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
