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

test('health lists checks and counts failures and warnings for the tab badge', () => {
  const health = { checks: [{ name: 'store', status: 'ok', detail: 'audit passed' }, { name: 'maintenance', status: 'fail', detail: 'auth_failed' }, { name: 'codex_trust', status: 'warn', detail: 'confirm' }], guards: [{ guard_id: 'g_1', status: 'fail', failure: 'x' }] };
  assert.match(yard.healthHTML(health), /auth_failed/);
  assert.equal(yard.attentionCount(health), 3);
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

test('the attention badge is set without visiting Health', async () => {
  let attention = null;
  const container = {
    querySelectorAll() { return []; },
    querySelector() { return { innerHTML: '', addEventListener() {} }; },
    set innerHTML(_html) {},
  };
  const fetchJSON = (url) => Promise.resolve(url === '/api/preferences/health' ? { checks: [{ status: 'fail', name: 'x', detail: '' }] } : []);
  await yard.render(container, { fetchJSON, onAttention: (n) => { attention = n; } });
  await new Promise((r) => setImmediate(r));
  assert.equal(attention, 1);
});

test('pipeline columns list their rules; captured text is trimmed with the full text on hover', () => {
  const long = 'if state is not latest and has been updated can we show a refresh button like github does? '.repeat(4);
  const rules = [
    { id: 'r1', directive: 'Prefer clear names.', status: 'candidate', scope_kind: 'global', surfaces: ['naming'] },
    { id: 'r2', directive: 'Keep tests local.', status: 'active', scope_kind: 'repository', repo_key: 'orion-a1b2c3d4e5f6', surfaces: [] },
    { id: 'r3', directive: 'Core rule.', status: 'core', scope_kind: 'global', surfaces: [] },
    { id: 'r4', directive: 'Old rule.', status: 'superseded', scope_kind: 'global', surfaces: [] },
  ];
  const html = yard.pipelineHTML({ captured: [{ id: 'o1', source: 'review', excerpt: long, captured_at: '2026-10-31T10:00:00Z' }], candidate: 1, active: 2, retired: 1 }, rules);
  const column = name => html.split('<section class="yard-column"').find(c => c.includes('<span>' + name + '</span>'));
  assert.match(column('Candidate'), /Prefer clear names\./);
  assert.match(column('Active'), /Keep tests local\./);
  assert.match(column('Active'), /Core rule\./);
  assert.match(column('Retired'), /Old rule\./);
  assert.doesNotMatch(column('Active'), /Old rule\./);
  assert.match(column('Captured'), /class="yard-card-text yard-clamp" title="if state is not latest/);
});

test('persona files render as markdown without the generated header', () => {
  const files = { 'core.md': '<!-- Generated by hunkyard-preferences; do not edit. -->\n# Core\n\n- Apply naming-analyzer\n' };
  const render = md => '<rendered>' + md + '</rendered>';
  const html = yard.personaHTML(files, 'core.md', render);
  assert.match(html, /<article class="yard-file-view yard-markdown"><rendered># Core/);
  assert.doesNotMatch(html, /Generated by/);
  // Without a renderer the file stays readable as text.
  assert.match(yard.personaHTML(files, 'core.md'), /<pre class="yard-file-view">/);
});

test('a slow view never overwrites the view the user switched to', async () => {
  const body = { innerHTML: '', insertAdjacentHTML() {} };
  const listeners = {};
  const root = { addEventListener: (type, fn) => { listeners[type] = fn; } };
  const container = {
    set innerHTML(_) {},
    querySelector: (sel) => (sel === '.yard-body' ? body : root),
    querySelectorAll: () => [],
  };
  const pending = {};
  const fetchJSON = (path) => new Promise((resolve) => { pending[path] = resolve; });
  yard.render(container, { fetchJSON });
  const click = (view) => listeners.click({ target: { closest: () => ({ hasAttribute: (a) => a === 'data-yard-view', getAttribute: () => view }) } });
  click('loads');
  click('health');
  pending['/api/preferences/health']({ checks: [] });
  await new Promise((r) => setImmediate(r));
  const healthHTML = body.innerHTML;
  pending['/api/preferences/loads']([]);
  await new Promise((r) => setImmediate(r));
  assert.equal(body.innerHTML, healthHTML, 'the earlier Loads response replaced Health');
});

test('the view switcher is a tablist with one selected, focusable tab', () => {
  const html = yard.tabsHTML('rules');
  assert.match(html, /role="tablist"/);
  assert.equal((html.match(/role="tab"/g) || []).length, 7);
  assert.match(html, /data-yard-view="rules"[^>]*aria-selected="true"[^>]*tabindex="0"|aria-selected="true"[^>]*tabindex="0"[^>]*data-yard-view="rules"/);
  assert.equal((html.match(/aria-selected="true"/g) || []).length, 1);
  assert.equal((html.match(/tabindex="-1"/g) || []).length, 6, 'roving tabindex: only the selected tab is in the Tab order');
  assert.match(html, /aria-controls="yard-panel"/);
});

test('rules search matches directive, surfaces and repository, and the toolbar counts what is shown', () => {
  const rules = [
    { id: 'a', directive: 'Prefer early returns.', status: 'active', scope_kind: 'global', surfaces: ['ruby'] },
    { id: 'b', directive: 'Keep fixtures small.', status: 'active', scope_kind: 'repository', repo_key: 'orion-a1b2c3d4e5f6', surfaces: ['testing'] },
    { id: 'c', directive: 'Old idea.', status: 'candidate', scope_kind: 'global', surfaces: [] },
  ];
  assert.deepEqual(yard.filterRules(rules, { status: 'active', query: 'FIXTURE' }).map(r => r.id), ['b']);
  assert.deepEqual(yard.filterRules(rules, { status: 'active', query: 'orion' }).map(r => r.id), ['b']);
  assert.deepEqual(yard.filterRules(rules, { status: 'active', query: 'ruby' }).map(r => r.id), ['a']);
  const bar = yard.rulesToolbarHTML(rules, { status: 'active' });
  assert.match(bar, /2 active rules/);
  assert.match(bar, /type="search"[^>]*aria-label="Search rules"/);
  assert.match(bar, /aria-label="Status"/);
  assert.match(bar, /aria-label="Scope"/);
  assert.match(bar, /aria-label="Surface"/);
  assert.match(bar, /<option value="testing"/, 'surface filter lists the surfaces in use');
});

test('long rule lists render in batches with a Show more control', () => {
  const rules = Array.from({ length: 120 }, (_, i) => ({ id: 'r' + i, directive: 'Rule ' + i, status: 'active', scope_kind: 'global', surfaces: [] }));
  const first = yard.rulesHTML(rules, { status: 'active' }, 50);
  assert.equal((first.match(/<li class="yard-row/g) || []).length, 50);
  assert.match(first, /data-yard-action="more"[^>]*>Show 50 more/);
  const all = yard.rulesHTML(rules, { status: 'active' }, 150);
  assert.equal((all.match(/<li class="yard-row/g) || []).length, 120);
  assert.doesNotMatch(all, /data-yard-action="more"/);
});

test('a rule title is not bold and the meta line does not repeat its status', () => {
  const html = yard.rulesHTML([{ id: 'a', directive: 'Prefer early returns.', status: 'active', scope_kind: 'global', surfaces: [], observations: 2, origin: 'feedback', updated_at: '2026-10-31T10:00:00Z' }], { status: 'active' });
  assert.doesNotMatch(html, /yard-row-meta">active ·/);
  assert.match(html, /2 observations/);
  assert.match(html, /class="yard-dot yard-dot--active" role="img" aria-label="Active"/);
});

test('Retire asks for confirmation before it acts', () => {
  const html = yard.rulesHTML([{ id: 'a', directive: 'x', status: 'active', scope_kind: 'global', surfaces: [] }], { status: 'active' }, 50, 'a');
  assert.match(html, /data-yard-action="retire"[^>]*data-confirmed="true"[^>]*>Confirm retire/);
  const plain = yard.rulesHTML([{ id: 'a', directive: 'x', status: 'active', scope_kind: 'global', surfaces: [] }], { status: 'active' });
  assert.match(plain, /data-yard-action="retire"[^>]*>Retire</);
  assert.doesNotMatch(plain, /data-confirmed/);
});

test('Run maintenance lives in the Pipeline toolbar, on the right, and explains when there is nothing to do', () => {
  const ready = yard.pipelineToolbarHTML({ captured: [{ id: 'o1' }, { id: 'o2' }], candidate: 3 });
  assert.match(ready, /class="yard-tools-end"[\s\S]*data-yard-action="maintenance"/);
  assert.match(ready, /Consolidate 2 captured · 3 candidates/);
  assert.doesNotMatch(ready, /disabled/);
  const idle = yard.pipelineToolbarHTML({ captured: [], candidate: 0 });
  assert.match(idle, /data-yard-action="maintenance"[^>]*disabled/);
  assert.match(idle, /Nothing waiting for maintenance/);
  assert.doesNotMatch(yard.pipelineHTML({ captured: [{ id: 'o1', excerpt: 'x' }], candidate: 1, active: 0, retired: 0 }, []), /data-yard-action="maintenance"/, 'the board itself no longer carries the button');
});

test('health: warnings count toward the badge and a guard with no examples is not a pass', () => {
  assert.equal(yard.attentionCount({ checks: [{ status: 'warn' }, { status: 'ok' }], guards: [{ status: 'fail' }] }), 2);
  const html = yard.healthHTML({ checks: [{ name: 'codex_trust', status: 'warn', detail: 'untrusted' }], guards: [{ kind: 'staged_paths', status: 'pass', examples: 0 }] });
  assert.match(html, /yard-dot--warn" role="img" aria-label="Warning"/);
  assert.match(html, /yard-dot--none" role="img" aria-label="No examples"/);
  assert.match(html, /No examples yet/);
});

test('a load error is an alert with a retry, not an empty state', () => {
  const html = yard.errorHTML('Could not load Rules: 500');
  assert.match(html, /role="alert"/);
  assert.match(html, /class="yard-error"/);
  assert.match(html, /data-yard-action="retry"/);
});

test('persona headings keep their levels', () => {
  const html = yard.personaHTML({ 'core.md': '# Title\n\n## Section\n' }, 'core.md', (t) => t.replace(/^# (.*)$/m, '<h1>$1</h1>').replace(/^## (.*)$/m, '<h2>$1</h2>'));
  assert.match(html, /<h1>Title<\/h1>/);
  assert.match(html, /class="yard-tree" aria-label="Persona files"/);
});

test('the attention count shows on the Health tab, and Health says what needs attention first', () => {
  const tabs = yard.tabsHTML('review', 2);
  assert.match(tabs, /data-yard-view="health"[^>]*>Health<span class="yard-tab-badge" aria-label="2 need attention">2<\/span>/);
  assert.doesNotMatch(yard.tabsHTML('review', 0), /yard-tab-badge/);
  const html = yard.healthHTML({ checks: [{ name: 'store', status: 'ok', detail: 'fine' }, { name: 'codex_trust', status: 'warn', detail: 'confirm' }, { name: 'claude_hooks', status: 'fail', detail: 'missing' }] });
  assert.match(html, /class="yard-health-summary needs-attention">2 need attention/);
  assert.ok(html.indexOf('claude hooks') < html.indexOf('codex trust') && html.indexOf('codex trust') < html.indexOf('store'), 'failing, then warnings, then passing');
  assert.match(yard.healthHTML({ checks: [{ name: 'store', status: 'ok', detail: 'fine' }] }), /class="yard-health-summary">Everything is healthy/);
});
