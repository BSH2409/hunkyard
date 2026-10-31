const { describe, it, beforeEach } = require('node:test');
const assert = require('node:assert/strict');

// Minimal DOM mock
class MockElement {
  constructor(tag) {
    this.tagName = tag;
    this.className = '';
    this.textContent = '';
    this.innerHTML = '';
    this.placeholder = '';
    this.type = '';
    this.title = '';
    this.value = '';
    this.dataset = {};
    this.children = [];
    this.listeners = {};
    this.parentNode = null;
  }
  appendChild(child) { this.children.push(child); child.parentNode = this; return child; }
  insertBefore(child, ref) { const i = this.children.indexOf(ref); if (i >= 0) this.children.splice(i, 0, child); else this.children.push(child); child.parentNode = this; return child; }
  remove() { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter(c => c !== this); }
  addEventListener(type, fn) { if (!this.listeners[type]) this.listeners[type] = []; this.listeners[type].push(fn); }
  dispatchEvent(e) { (this.listeners[e.type] || []).forEach(fn => fn(e)); }
  focus() {}
}

global.document = {
  createElement: (tag) => new MockElement(tag),
};
global.window = { hunkyard: {} };
global.requestAnimationFrame = (fn) => fn();

const { createForm } = require('../hunkyard-comment-form.js');

describe('hunkyard-comment-form', () => {
  it('createForm returns object with el, focus, getBody, setBody, destroy', () => {
    const f = createForm({ formKey: 'test' });
    assert.ok(f.el);
    assert.equal(typeof f.focus, 'function');
    assert.equal(typeof f.getBody, 'function');
    assert.equal(typeof f.setBody, 'function');
    assert.equal(typeof f.destroy, 'function');
  });

  it('initialBody pre-fills textarea', () => {
    const f = createForm({ initialBody: 'hello world' });
    assert.equal(f.getBody(), 'hello world');
  });

  it('setBody updates textarea value', () => {
    const f = createForm({});
    f.setBody('new value');
    assert.equal(f.getBody(), 'new value');
  });

  it('Ctrl+Enter calls onSubmit with trimmed body', () => {
    let submitted = null;
    const f = createForm({ onSubmit: (body) => { submitted = body; } });
    f.setBody('  test body  ');
    const textarea = f.getTextarea();
    textarea.dispatchEvent({ type: 'keydown', key: 'Enter', ctrlKey: true, preventDefault: () => {} });
    assert.equal(submitted, 'test body');
  });

  it('empty body does not call onSubmit', () => {
    let called = false;
    const f = createForm({ onSubmit: () => { called = true; } });
    const textarea = f.getTextarea();
    textarea.dispatchEvent({ type: 'keydown', key: 'Enter', ctrlKey: true, preventDefault: () => {} });
    assert.equal(called, false);
  });

  it('onInput called on textarea input event', () => {
    let received = null;
    const f = createForm({ onInput: (v) => { received = v; } });
    f.setBody('typing');
    f.getTextarea().dispatchEvent({ type: 'input' });
    assert.equal(received, 'typing');
  });

  it('wrapper has comment-form-wrapper class', () => {
    const f = createForm({});
    assert.ok(f.el.className.includes('comment-form-wrapper'));
  });

  it('compact mode adds compact class', () => {
    const f = createForm({ compact: true });
    assert.ok(f.el.className.includes('compact'));
  });
});

describe('label picker', () => {
  const { buildLabelPicker } = require('../hunkyard-comment-form.js');
  function click(el) { (el.listeners.click || []).forEach(fn => fn({ preventDefault() {} })); }
  it('selects one label at a time and toggles blocking', () => {
    const picker = buildLabelPicker();
    assert.deepEqual(picker.getValue(), { label: '', blocking: false });
    const buttons = picker.el.children;
    const issue = buttons.find(b => b.dataset.label === 'issue');
    const nit = buttons.find(b => b.dataset.label === 'nit');
    const blocking = buttons.find(b => b.dataset.blocking === 'true');
    click(issue);
    click(blocking);
    assert.deepEqual(picker.getValue(), { label: 'issue', blocking: true });
    click(nit);
    assert.equal(picker.getValue().label, 'nit');
    click(nit);
    assert.equal(picker.getValue().label, '');
  });
});
