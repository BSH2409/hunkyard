'use strict';
// Viewed state stores the file_hash seen when a file was marked viewed.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const appJs = fs.readFileSync(path.join(__dirname, '..', 'app.js'), 'utf8');

function extractFunction(name) {
  const start = appJs.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} must exist`);
  const bodyStart = appJs.indexOf('{', start);
  let depth = 0;
  for (let i = bodyStart; i < appJs.length; i++) {
    if (appJs[i] === '{') depth++;
    if (appJs[i] === '}') {
      depth--;
      if (depth === 0) return appJs.slice(start, i + 1);
    }
  }
  throw new Error(`could not extract ${name}`);
}

const { viewedRecord, viewedStateFor } = new Function(`
  ${extractFunction('viewedRecord')}
  ${extractFunction('viewedStateFor')}
  return { viewedRecord, viewedStateFor };
`)();

test('a lazy file without a hash keeps the hash stored earlier', () => {
  assert.equal(viewedRecord({ viewed: true, fileHash: '' }, 'sha256:old'), 'sha256:old');
  assert.equal(viewedRecord({ viewed: true, fileHash: '' }, undefined), true);
  assert.equal(viewedRecord({ viewed: true, fileHash: 'sha256:new' }, 'sha256:old'), 'sha256:new');
  assert.equal(viewedRecord({ viewed: false, changedSinceViewed: true, viewedHash: 'sha256:old' }, 'sha256:old'), 'sha256:old');
  assert.equal(viewedRecord({ viewed: false }, 'sha256:old'), undefined);
});

test('a stored hash that no longer matches un-views the file', () => {
  assert.deepEqual(viewedStateFor('sha256:old', 'sha256:new'), { viewed: false, changed: true });
  assert.deepEqual(viewedStateFor('sha256:same', 'sha256:same'), { viewed: true, changed: false });
  assert.deepEqual(viewedStateFor(true, 'sha256:any'), { viewed: true, changed: false });
  assert.deepEqual(viewedStateFor('sha256:old', ''), { viewed: true, changed: false });
  assert.deepEqual(viewedStateFor(undefined, 'sha256:x'), { viewed: false, changed: false });
});
