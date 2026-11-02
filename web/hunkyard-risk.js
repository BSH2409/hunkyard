// Risk ordering for the file list: an explainable score per changed file
// (sensitive path, size of the change, no test changed, uncovered lines,
// rule findings). Pure; app.js sorts with compare().
(function () {
  'use strict';

  var defaultPatterns = ['**/auth/**', '**/*secret*', '**/*password*', '**/*token*', '**/migrations/**', '**/*payment*', '**/*permission*', '**/*.sql'];

  // globToRegExp: ** spans directories (including none), * stays in one segment.
  function globToRegExp(glob) {
    var out = '';
    for (var i = 0; i < glob.length; i++) {
      var ch = glob[i];
      if (ch === '*' && glob[i + 1] === '*') {
        var slash = glob[i + 2] === '/';
        out += slash ? '(?:.*/)?' : '.*';
        i += slash ? 2 : 1;
      } else if (ch === '*') out += '[^/]*';
      else if (ch === '?') out += '[^/]';
      else out += ch.replace(/[.+^${}()|[\]\\]/g, '\\$&');
    }
    return new RegExp('^' + out + '$', 'i');
  }

  var cache = {};
  function matches(path, patterns) {
    for (var i = 0; i < (patterns || []).length; i++) {
      var p = patterns[i];
      var re = cache[p] || (cache[p] = globToRegExp(p));
      if (re.test(path)) return true;
    }
    return false;
  }

  function isTestPath(path) {
    var base = path.split('/').pop();
    return /_test\.go$|\.spec\.|\.test\.|_spec\.rb$|_test\.py$|^test_/.test(base) || /(^|\/)(test|tests|spec|__tests__)\//.test(path);
  }

  function stem(path) {
    return path.split('/').pop().replace(/(_test|_spec|\.spec|\.test)?\.[^.]+$/, '').replace(/^test_/, '');
  }

  // A file counts as tested when a changed test shares its directory or stem.
  function hasChangedTest(path, changedPaths) {
    var dir = path.slice(0, path.lastIndexOf('/') + 1);
    var name = stem(path);
    return (changedPaths || []).some(function (p) {
      return isTestPath(p) && (stem(p) === name || p.slice(0, p.lastIndexOf('/') + 1) === dir);
    });
  }

  // score(file, ctx) → { value, reasons }. ctx: { patterns, changedPaths,
  // coverage: {uncovered}, ruleFindings }.
  function score(file, ctx) {
    var value = 0;
    var reasons = [];
    if (matches(file.path, ctx.patterns)) { value += 3; reasons.push('sensitive path'); }
    var size = (file.additions || 0) + (file.deletions || 0);
    if (size > 0) {
      var sizeScore = Math.ceil(Math.log2(size + 1));
      value += sizeScore;
      reasons.push(size + ' changed lines');
    }
    if (!isTestPath(file.path) && !hasChangedTest(file.path, ctx.changedPaths)) { value += 2; reasons.push('no test changed for it'); }
    if (ctx.coverage && ctx.coverage.uncovered > 0) { value += 2; reasons.push(ctx.coverage.uncovered + ' uncovered lines'); }
    if (ctx.ruleFindings > 0) { value += Math.min(3, ctx.ruleFindings); reasons.push(ctx.ruleFindings + ' rule findings'); }
    return { value: value, reasons: reasons };
  }

  // compare sorts by score (highest first), then by path.
  function compare(scores, pathCompare) {
    return function (a, b) {
      var sa = (scores.get(a.path) || { value: 0 }).value;
      var sb = (scores.get(b.path) || { value: 0 }).value;
      return sb - sa || pathCompare(a.path, b.path);
    };
  }

  var api = { defaultPatterns: defaultPatterns, score: score, compare: compare };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.risk = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
