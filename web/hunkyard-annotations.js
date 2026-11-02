// Tool annotations shown in the review: coverage marks imported by
// `hunkyard annotate`.
// Pure helpers; app.js fetches /api/annotations and applies them.
(function () {
  'use strict';

  function coverageLine(coverage) {
    if (!coverage || !coverage.changed) return '';
    var noun = coverage.changed === 1 ? 'line' : 'lines';
    if (!coverage.uncovered) return 'All ' + coverage.changed + ' changed ' + noun + ' covered';
    return coverage.uncovered + ' of ' + coverage.changed + ' changed ' + noun + ' uncovered';
  }

  function currentTools(tools, round) {
    return Object.keys(tools || {}).sort().map(function (name) { return tools[name]; })
      .filter(function (t) { return t && t.round === round; });
  }

  // Uncovered line ranges of one file across every tool of this round,
  // sorted and merged where they touch.
  function uncoveredRanges(tools, path, round) {
    var ranges = [];
    currentTools(tools, round).forEach(function (t) {
      ((t.marks || {})[path] || []).forEach(function (m) {
        if (m.kind === 'uncovered') ranges.push({ start: m.start, end: m.end });
      });
    });
    ranges.sort(function (a, b) { return a.start - b.start; });
    var merged = [];
    ranges.forEach(function (r) {
      var last = merged[merged.length - 1];
      if (last && r.start <= last.end + 1) last.end = Math.max(last.end, r.end);
      else merged.push({ start: r.start, end: r.end });
    });
    return merged;
  }

  // Coverage of one file this round; the tool reporting the most uncovered
  // lines wins when several report coverage.
  function fileCoverage(tools, path, round) {
    var best = null;
    currentTools(tools, round).forEach(function (t) {
      var c = (t.coverage || {})[path];
      if (c && (!best || c.uncovered > best.uncovered)) best = { changed: c.changed, uncovered: c.uncovered };
    });
    return best;
  }

  var api = { coverageLine: coverageLine, uncoveredRanges: uncoveredRanges, fileCoverage: fileCoverage };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.annotations = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
