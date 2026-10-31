'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const yard = require('../hunkyard-yard.js');

const rules = [
  { id: 'rule_a', directive: 'Write <Ruby> defs over multiple lines.', status: 'active', origin: 'feedback', scope_kind: 'global', surfaces: ['ruby'], observations: 3, violations: 2, updated_at: '2026-10-31T10:00:00Z' },
  { id: 'rule_b', directive: 'Keep tests local.', status: 'active', origin: 'feedback', scope_kind: 'repository', repo_key: 'orion-a1b2c3d4e5f6', surfaces: ['testing'], observations: 1, violations: 0, updated_at: '2026-10-30T10:00:00Z' },
  { id: 'rule_c', directive: 'Old rule.', status: 'superseded', origin: 'inferred', scope_kind: 'global', surfaces: ['naming'], observations: 1, violations: 0, updated_at: '2026-10-01T10:00:00Z' },
];

test('rules list escapes directives and shows scope, surface and violation labels', () => {
  const html = yard.rulesHTML(rules, { status: 'active' });
  assert.match(html, /Write &lt;Ruby&gt; defs/);
  assert.match(html, /yard-label--global">global</);
  assert.match(html, /yard-label--repo">orion</);
  assert.match(html, /2 violations/);
  assert.doesNotMatch(html, /Old rule/);
  assert.match(html, /data-yard-action="retire" data-rule-id="rule_a"/);
  assert.match(html, /data-yard-action="promote" data-rule-id="rule_b"/);
  assert.doesNotMatch(html, /data-yard-action="promote" data-rule-id="rule_a"/);
});

test('rules filter by scope and surface', () => {
  assert.deepEqual(yard.filterRules(rules, { status: 'active', scope: 'repository' }).map(r => r.id), ['rule_b']);
  assert.deepEqual(yard.filterRules(rules, { status: 'all', surface: 'naming' }).map(r => r.id), ['rule_c']);
});

test('empty views explain what will appear', () => {
  assert.match(yard.rulesHTML([], {}), /yard-empty/);
  assert.match(yard.activityHTML([]), /yard-empty/);
  assert.match(yard.personaHTML({}, ''), /yard-empty/);
});

test('pipeline board shows counts and captured cards', () => {
  const html = yard.pipelineHTML({ captured: [{ id: 'obs_1', source: 'review', repo_key: 'blogvault-10d0d94c9194', excerpt: 'we dont use single line definition', captured_at: '2026-10-31T10:00:00Z' }], candidate: 72, active: 207, retired: 41 });
  assert.match(html, /Captured<\/span><span class="yard-count">1</);
  assert.match(html, /Active<\/span><span class="yard-count">207</);
  assert.match(html, /single line definition/);
});

test('persona tree lists files and shows the selected one', () => {
  const html = yard.personaHTML({ 'core.md': '- Apply naming-analyzer', 'repositories/orion-a1b2c3d4e5f6/core.md': '- Keep tests local' }, 'core.md');
  assert.match(html, /data-persona-file="core.md"/);
  assert.match(html, /repositories\/orion/);
  assert.match(html, /Apply naming-analyzer/);
});

test('health lists checks and counts failures for the tab badge', () => {
  const health = { checks: [{ name: 'store', status: 'ok', detail: 'audit passed' }, { name: 'maintenance', status: 'fail', detail: 'auth_failed' }, { name: 'codex_trust', status: 'warn', detail: 'confirm' }], guards: [{ guard_id: 'g_1', status: 'fail', failure: 'x' }] };
  assert.match(yard.healthHTML(health), /auth_failed/);
  assert.equal(yard.attentionCount(health), 2);
});

test('activity timeline labels kinds', () => {
  const html = yard.activityHTML([{ at: '2026-10-31T10:00:00Z', kind: 'blocked', rule_id: 'rule_a', detail: 'the commit includes plan.md' }]);
  assert.match(html, /yard-event--blocked/);
  assert.match(html, /plan\.md/);
});

test('loads view shows each session rule with why it was loaded', () => {
  assert.match(yard.loadsHTML([]), /yard-empty/);
  const html = yard.loadsHTML([{ session: 'abc', updated_at: '2026-10-31T10:00:00Z', rules: [{ id: 'rule_a', directive: 'Use <b>info</b> logs', reason: 'matched "ruby" in your prompt' }] }]);
  assert.match(html, /Use &lt;b&gt;info&lt;\/b&gt; logs/);
  assert.match(html, /matched &quot;ruby&quot; in your prompt/);
});

test('re-rendering into the same pane does not stack listeners', async () => {
  let containerListeners = 0;
  const roots = [];
  const body = { innerHTML: '' };
  const container = {
    addEventListener() { containerListeners++; },
    querySelectorAll() { return []; },
    querySelector(selector) {
      if (selector === '.yard-body') return body;
      return roots[roots.length - 1];
    },
    set innerHTML(_html) { roots.push({ listeners: 0, addEventListener() { this.listeners++; } }); },
  };
  const opts = { fetchJSON: () => Promise.resolve([]), postJSON: () => Promise.resolve({}) };
  await yard.render(container, opts);
  await yard.render(container, opts);
  assert.equal(containerListeners, 0);
  assert.equal(roots.length, 2);
  assert.ok(roots[1].listeners > 0);
});
