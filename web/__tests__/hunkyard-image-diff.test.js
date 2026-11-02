'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');

// Minimal DOM: elements with attributes, children, classList, listeners.
function el(tag) {
  const node = {
    tagName: tag.toUpperCase(), children: [], attrs: {}, dataset: {}, style: {}, listeners: {}, hidden: false, textContent: '',
    classList: { set: new Set(), add(c) { this.set.add(c); }, contains(c) { return this.set.has(c); }, toggle(c, on) { if (on) this.set.add(c); else this.set.delete(c); } },
    set className(v) { v.split(/\s+/).filter(Boolean).forEach((c) => this.classList.add(c)); },
    get className() { return Array.from(this.classList.set).join(' '); },
    appendChild(c) { this.children.push(c); c.parentNode = node; return c; },
    setAttribute(k, v) { this.attrs[k] = String(v); },
    getAttribute(k) { return this.attrs[k]; },
    addEventListener(name, fn) { (this.listeners[name] = this.listeners[name] || []).push(fn); },
    fire(name, event) { (this.listeners[name] || []).forEach((fn) => fn(event || { target: node })); },
  };
  return node;
}
global.document = { createElement: el };
const imageDiff = require('../hunkyard-image-diff.js');

function all(root, predicate, out = []) {
  if (predicate(root)) out.push(root);
  root.children.forEach((c) => all(c, predicate, out));
  return out;
}
const imgs = (root) => all(root, (n) => n.tagName === 'IMG');
const buttons = (root) => all(root, (n) => n.tagName === 'BUTTON');

test('isImagePath recognises the reviewed image types', () => {
  for (const p of ['a.png', 'b/c.JPG', 'd.jpeg', 'e.gif', 'f.webp']) assert.equal(imageDiff.isImagePath(p), true, p);
  for (const p of ['a.svg', 'b.bmp', 'c.png.txt', '']) assert.equal(imageDiff.isImagePath(p), false, p);
});

test('a modified image shows base and head side by side with a mode toggle', () => {
  const root = imageDiff.build({ path: 'a.png', status: 'modified', baseUrl: '/b', headUrl: '/h' });
  assert.equal(root.dataset.mode, 'side');
  assert.deepEqual(imgs(root).map((i) => i.attrs.src), ['/b', '/h']);
  assert.deepEqual(buttons(root).map((b) => b.textContent), ['Side by side', 'Slider', 'Onion skin']);
  buttons(root)[1].fire('click');
  assert.equal(root.dataset.mode, 'slider');
  assert.equal(buttons(root)[1].attrs['aria-pressed'], 'true');
});

test('added and deleted images show their one side without a toggle', () => {
  const added = imageDiff.build({ path: 'a.png', status: 'added', baseUrl: '/b', headUrl: '/h' });
  assert.deepEqual(imgs(added).map((i) => i.attrs.src), ['/h']);
  assert.equal(buttons(added).length, 0);
  const deleted = imageDiff.build({ path: 'a.png', status: 'deleted', baseUrl: '/b', headUrl: '/h' });
  assert.deepEqual(imgs(deleted).map((i) => i.attrs.src), ['/b']);
});

test('a broken image falls back once', () => {
  let fallbacks = 0;
  const root = imageDiff.build({ path: 'a.png', status: 'modified', baseUrl: '/b', headUrl: '/h', onFallback: () => { fallbacks++; } });
  imgs(root)[0].fire('error');
  imgs(root)[1].fire('error');
  assert.equal(fallbacks, 1);
});

