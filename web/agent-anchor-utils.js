'use strict';
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.hunkyard = root.hunkyard || {};
    root.hunkyard.agent = root.hunkyard.agent || {};
    root.hunkyard.agent.anchorUtils = api;
  }
})(typeof window !== 'undefined' ? window : globalThis, function () {
  const IMPLICIT_ROLES = {
    A: 'link', AREA: 'link',
    BUTTON: 'button',
    NAV: 'navigation',
    MAIN: 'main',
    HEADER: 'banner',
    FOOTER: 'contentinfo',
    ASIDE: 'complementary',
    SECTION: 'region',
    ARTICLE: 'article',
    H1: 'heading', H2: 'heading', H3: 'heading',
    H4: 'heading', H5: 'heading', H6: 'heading',
    UL: 'list', OL: 'list',
    LI: 'listitem',
    IMG: 'img',
    INPUT: 'textbox',
    SELECT: 'combobox',
    TEXTAREA: 'textbox',
    FORM: 'form',
    TABLE: 'table',
    THEAD: 'rowgroup', TBODY: 'rowgroup', TFOOT: 'rowgroup',
    TR: 'row',
    TH: 'columnheader',
    TD: 'cell',
    DIALOG: 'dialog',
  };
  function implicitRole(tagName) {
    if (typeof tagName !== 'string') return '';
    return IMPLICIT_ROLES[tagName.toUpperCase()] || '';
  }

  // Resolution risk: nearest-id semantics may pick a parent whose id is reused or
  // later renamed in the user app, breaking the css_selector across page redeploys.
  // Phase D drift detection re-resolves selections using `tag_chain` +
  // `accessible_name` + `landmark` as fallback fields when the selector misses.
  function findAnchorRoot(el) {
    let cur = el;
    while (cur) {
      if (cur.id) return cur;
      if (cur.tagName === 'BODY') return cur;
      cur = cur.parentNode;
    }
    return el;
  }

  function indexOfType(el) {
    if (!el.parentNode) return 1;
    let n = 0;
    for (const sib of el.parentNode.children) {
      if (sib.tagName === el.tagName) {
        n += 1;
        if (sib === el) return n;
      }
    }
    return n;
  }

  function pathFromRoot(el, root) {
    const chain = [];
    let cur = el;
    while (cur && cur !== root) {
      chain.unshift(cur);
      cur = cur.parentNode;
    }
    if (cur !== root) return [root]; // detached: return root only
    return [root, ...chain];
  }

  function cssSelectorFor(el, root) {
    const chain = pathFromRoot(el, root);
    const head = chain[0];
    const headSel = head.id ? `#${head.id}` : head.tagName.toLowerCase();
    const tail = chain.slice(1).map(node => `${node.tagName.toLowerCase()}:nth-of-type(${indexOfType(node)})`);
    return [headSel, ...tail].join(' > ');
  }

  function tagChainFor(el, root) {
    return pathFromRoot(el, root).map(node => node.tagName.toUpperCase());
  }

  function accessibleNameFor(el) {
    // Order: explicit `aria-label` attribute first, then the `ariaLabel` IDL
    // property, finally the rendered text (innerText keeps table cells and
    // lines apart; textContent runs them together). All capped at 80.
    const attr = (el.getAttribute && el.getAttribute('aria-label')) || '';
    if (attr) return String(attr).trim().slice(0, 80);
    const idl = el.ariaLabel || '';
    if (idl) return String(idl).trim().slice(0, 80);
    const text = typeof el.innerText === 'string' && el.innerText ? el.innerText : (el.textContent || '');
    return text.replace(/\s+/g, ' ').trim().slice(0, 80);
  }

  function roleFor(el) {
    const explicit = el.getAttribute && el.getAttribute('role');
    if (explicit) return explicit;
    return implicitRole(el.tagName);
  }

  const LANDMARK_TAGS = new Set(['MAIN', 'NAV', 'HEADER', 'FOOTER', 'SECTION', 'ASIDE']);

  function landmarkFor(el) {
    let cur = el.parentNode;
    while (cur) {
      if (LANDMARK_TAGS.has(cur.tagName)) {
        const aria = cur.ariaLabel || (cur.getAttribute && cur.getAttribute('aria-label'));
        return aria || cur.tagName.toLowerCase();
      }
      cur = cur.parentNode;
    }
    return '';
  }

  function truncateOuterHTML(html, max) {
    if (typeof html !== 'string') return '';
    return html.length > max ? html.slice(0, max) : html;
  }

  function walkAncestors(el, root) {
    const out = [];
    let cur = el;
    while (cur) {
      out.push(cur);
      if (cur === root) break;
      cur = cur.parentNode;
    }
    return out;
  }

  // ---- Phase D: pin resolution ----

  function verifyTagChain(el, chain) {
    // chain: ["MAIN", "SECTION", "H2"]  (root-most first, target last)
    if (!el || !Array.isArray(chain) || chain.length === 0) return false;
    const ancestry = [];
    let cur = el;
    while (cur) { ancestry.push(cur.tagName); cur = cur.parentElement || cur.parentNode || null; }
    if (ancestry.length < chain.length) return false;
    for (let i = 0; i < chain.length; i++) {
      const fromAncestry = ancestry[chain.length - 1 - i];
      if (fromAncestry !== chain[i]) return false;
    }
    return true;
  }

  const LANDMARK_SELECTOR =
    'main, nav, header, footer, section, aside, ' +
    '[role="main"], [role="navigation"], [role="banner"], [role="contentinfo"], [role="region"], [role="complementary"]';

  function findLandmarkElement(doc, landmark) {
    if (!landmark || !doc || typeof doc.querySelectorAll !== 'function') return null;
    const candidates = doc.querySelectorAll(LANDMARK_SELECTOR) || [];
    const target = String(landmark).toLowerCase();
    for (const el of candidates) {
      const label = (el.getAttribute && el.getAttribute('aria-label')) || '';
      if (label && label === landmark) return el;
      const tag = (el.tagName || '').toLowerCase();
      if (tag === target) return el;
    }
    return null;
  }

  function findByRoleAndName(landmarkEl, role, name, tagChain) {
    if (!landmarkEl || !role || !name) return { element: null, matchCount: 0 };
    let candidates;
    if (Array.isArray(tagChain) && tagChain.length > 0) {
      const leafTag = String(tagChain[tagChain.length - 1]).toLowerCase();
      candidates = landmarkEl.querySelectorAll(leafTag) || [];
    } else {
      candidates = landmarkEl.querySelectorAll('*') || [];
    }
    const matches = [];
    for (const el of candidates) {
      const elRole = roleFor(el);
      if (elRole !== role) continue;
      const elName = accessibleNameFor(el);
      if (elName === name) matches.push(el);
    }
    return { element: matches[0] || null, matchCount: matches.length };
  }

  function resolvePin(anchor, doc) {
    if (!anchor || !doc) return { status: 'drifted', element: null };
    let el = null;
    try { el = doc.querySelector(anchor.css_selector); } catch (e) { el = null; }
    if (el && verifyTagChain(el, anchor.tag_chain || [])) {
      return { status: 'resolved', element: el };
    }
    if (anchor.role && anchor.accessible_name && anchor.landmark) {
      const lm = findLandmarkElement(doc, anchor.landmark);
      if (lm) {
        const found = findByRoleAndName(lm, anchor.role, anchor.accessible_name, anchor.tag_chain);
        // Several elements share the role and name (the same icon button in
        // every row): any pick would be a guess, so the pin is not recovered.
        if (found.element && found.matchCount === 1) {
          return {
            status: 'drifted-recoverable',
            element: found.element,
            recovered_via: 'role+name+landmark',
            matchCount: found.matchCount,
          };
        }
      }
    }
    return { status: 'drifted', element: null };
  }

  const FINGERPRINT_TEXT_MAX = 500;
  const FINGERPRINT_CONTEXT = 32;

  function normaliseText(s) {
    return String(s || '').replace(/\s+/g, ' ').trim();
  }

  // FNV-1a, 32-bit, as 8 hex digits.
  function fnv1a(str) {
    let h = 0x811c9dc5;
    for (let i = 0; i < str.length; i++) {
      h ^= str.charCodeAt(i);
      h = Math.imul(h, 0x01000193) >>> 0;
    }
    return h.toString(16).padStart(8, '0');
  }

  // The page's own markup: Hunkyard's classes (pick highlight, markers) and
  // the empty class attribute removing them can leave are not part of it.
  function withoutHunkyardClasses(html) {
    return String(html || '').replace(/\sclass="([^"]*)"/g, function (_, value) {
      var kept = value.split(/\s+/).filter(function (c) { return c && c.indexOf('hunkyard-') !== 0; });
      return kept.length ? ' class="' + kept.join(' ') + '"' : '';
    });
  }

  // What the element looks like now: its text, the text either side of it in
  // its parent, its size at this viewport width, and a hash of its HTML.
  function elementFingerprint(el, win) {
    const full = normaliseText(el.textContent);
    const around = normaliseText(el.parentNode && el.parentNode.textContent);
    const at = full ? around.indexOf(full) : -1;
    const box = typeof el.getBoundingClientRect === 'function' ? el.getBoundingClientRect() : { width: 0, height: 0 };
    return {
      text: full.slice(0, FINGERPRINT_TEXT_MAX),
      prefix: at >= 0 ? around.slice(0, at).trimEnd().slice(-FINGERPRINT_CONTEXT) : '',
      suffix: at >= 0 ? around.slice(at + full.length).trimStart().slice(0, FINGERPRINT_CONTEXT) : '',
      width: Math.round(box.width),
      height: Math.round(box.height),
      viewport_width: win.innerWidth,
      html_hash: fnv1a(normaliseText(withoutHunkyardClasses(el.outerHTML))),
    };
  }

  // 'current' when the pin's own selector finds an element that still looks
  // as it did when pinned; 'outdated' otherwise. Size counts only at the
  // viewport width it was measured at.
  function comparePin(anchor, resolution, win) {
    if (!resolution || resolution.status !== 'resolved' || !resolution.element) return 'outdated';
    const was = anchor && anchor.fingerprint;
    if (!was) return 'current';
    const now = elementFingerprint(resolution.element, win);
    if (now.text !== was.text || now.prefix !== was.prefix || now.suffix !== was.suffix || now.html_hash !== was.html_hash) {
      return 'outdated';
    }
    if (now.viewport_width === was.viewport_width && (now.width !== was.width || now.height !== was.height)) {
      return 'outdated';
    }
    return 'current';
  }

  return {
    implicitRole, findAnchorRoot, cssSelectorFor, tagChainFor,
    accessibleNameFor, roleFor, landmarkFor, truncateOuterHTML, walkAncestors,
    verifyTagChain, findLandmarkElement, findByRoleAndName, resolvePin,
    elementFingerprint, comparePin,
  };
});
