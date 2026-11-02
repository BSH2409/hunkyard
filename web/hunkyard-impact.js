// Impact panel: other files that mention the symbols a changed file's diff
// defines or removes (GET /api/file/impact). Text matches, not a call graph.
(function () {
  'use strict';

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function count(n, truncated, one, many) {
    if (!n) return 'no ' + many;
    return n + (truncated ? '+' : '') + ' ' + (n === 1 && !truncated ? one : many);
  }

  function summaryLine(result) {
    if (!result || !result.symbols || !result.symbols.length) return '';
    var t = result.truncated || {};
    var callers = count(result.callers.length, t.callers, 'caller', 'callers');
    var tests = count(result.tests.length, t.tests, 'test', 'tests');
    if (!result.tests.length) return callers + ' · ' + tests;
    var changed = result.tests.filter(function (x) { return x.changed; }).length;
    return callers + ' · ' + tests + ' · ' + changed + ' ' + (changed === 1 ? 'test' : 'tests') + ' changed';
  }

  // pillText is the header's short form: callers and tests only.
  function pillText(result) {
    if (!result || !result.symbols || !result.symbols.length) return '';
    var t = result.truncated || {};
    return count(result.callers.length, t.callers, 'caller', 'callers') + ' · ' + count(result.tests.length, t.tests, 'test', 'tests');
  }

  function panelHTML(result) {
    if (!result || !result.symbols || !result.symbols.length) {
      return '<div class="impact-panel"><p class="impact-empty">No changed definitions to look up in this file.</p></div>';
    }
    var head = '<p class="impact-symbols">Changed: <code>' + esc(result.symbols.join(', ')) + '</code> · text matches in tracked files</p>';
    if (!result.callers.length && !result.tests.length) {
      return '<div class="impact-panel">' + head + '<p class="impact-empty">No other file mentions these names.</p></div>';
    }
    var callers = result.callers.map(function (c) {
      return '<li><button type="button" class="impact-ref" data-impact-path="' + esc(c.path) + '" data-impact-line="' + c.line + '">' +
        esc(c.path) + ':' + c.line + '</button> <code class="impact-text">' + esc(c.text) + '</code></li>';
    }).join('');
    var tests = result.tests.map(function (x) {
      return '<li><button type="button" class="impact-ref" data-impact-path="' + esc(x.path) + '" data-impact-line="0">' + esc(x.path) + '</button>' +
        ' <span class="impact-hits">' + x.hits + ' mention' + (x.hits === 1 ? '' : 's') + '</span>' +
        (x.changed ? ' <span class="impact-changed">changed in this review</span>' : '') + '</li>';
    }).join('');
    var t = result.truncated || {};
    return '<div class="impact-panel">' + head +
      (callers ? '<h4>Callers' + (t.callers ? ' (first ' + result.callers.length + ')' : '') + '</h4><ul>' + callers + '</ul>' : '') +
      (tests ? '<h4>Tests' + (t.tests ? ' (first ' + result.tests.length + ')' : '') + '</h4><ul>' + tests + '</ul>' : '') +
      '</div>';
  }

  var api = { summaryLine: summaryLine, pillText: pillText, panelHTML: panelHTML };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.impact = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
