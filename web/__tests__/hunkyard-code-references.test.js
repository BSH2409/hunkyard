const { test } = require('node:test');
const assert = require('node:assert/strict');
const { references } = require('../hunkyard-code-references.js');

test('references identify reviewed paths and line ranges in prose and inline code', () => {
  const text = 'See `src/main.go:12-15`, @src/main.go#L20-L22 and ./README.md.';
  const matches = references(text, ['src/main.go', 'README.md']);
  assert.deepEqual(matches.map(({ path, line }) => ({ path, line })), [
    { path: 'src/main.go', line: 12 },
    { path: 'src/main.go', line: 20 },
    { path: 'README.md', line: 0 },
  ]);
  assert.equal(text.slice(matches[0].start, matches[0].end), 'src/main.go:12-15');
});

test('references reject unknown files and traversal paths', () => {
  assert.deepEqual(references('other.go:1 ../src/main.go:2 https://example.com', ['src/main.go']), []);
});
