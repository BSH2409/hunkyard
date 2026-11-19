(function(root) {
  'use strict';

  function references(text, paths) {
    const known = new Set(paths);
    const matches = [];
    const pattern = /@?([\w./-]+\.[\w-]+)(?::([1-9]\d*)(?:-[1-9]\d*)?|#L([1-9]\d*)(?:-L[1-9]\d*)?)?/g;
    let match;
    while ((match = pattern.exec(text))) {
      const path = match[1].replace(/^\.\//, '');
      if (!known.has(path)) continue;
      matches.push({ start: match.index, end: pattern.lastIndex, path: path, line: Number(match[2] || match[3] || 0) });
    }
    return matches;
  }

  function linkify(element, paths) {
    const doc = element.ownerDocument;
    const walker = doc.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    const nodes = [];
    let node;
    while ((node = walker.nextNode())) {
      if (!node.parentElement.closest('a, pre, .comment-ref, .code-reference')) nodes.push(node);
    }
    nodes.forEach(function(textNode) {
      const matches = references(textNode.textContent, paths);
      if (!matches.length) return;
      const fragment = doc.createDocumentFragment();
      let offset = 0;
      matches.forEach(function(match) {
        fragment.appendChild(doc.createTextNode(textNode.textContent.slice(offset, match.start)));
        const link = doc.createElement('span');
        link.className = 'code-reference';
        link.setAttribute('role', 'link');
        link.tabIndex = 0;
        link.dataset.path = match.path;
        link.dataset.line = String(match.line);
        link.textContent = textNode.textContent.slice(match.start, match.end);
        fragment.appendChild(link);
        offset = match.end;
      });
      fragment.appendChild(doc.createTextNode(textNode.textContent.slice(offset)));
      textNode.replaceWith(fragment);
    });
  }

  const api = { references: references, linkify: linkify };
  if (typeof module === 'object' && module.exports) module.exports = api;
  root.hunkyard = root.hunkyard || {};
  root.hunkyard.codeReferences = api;
})(typeof window === 'undefined' ? globalThis : window);
