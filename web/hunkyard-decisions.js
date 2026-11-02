// Questions from the agent (`hunkyard ask`), docked in the bottom-right corner
// of the review. Open questions are answered one page at a time: pick an
// option or write your own answer, then move on. A review page lists every
// question with your answer before you submit them together; nothing reaches
// the agent before Submit. Without open questions the dock shows past answers.
// It opens by itself for a question it has not shown yet; minimised, it is a
// launcher with the number of open questions. app.js fetches /api/decisions
// and sends the answers.
//
//   window.hunkyard.decisions.render(dockElement, decisionList, {
//     onSubmit: function (answers) { ... return promise; },  // [{id, answer: {chosen}|{text}|{dismiss: true}}]
//     onOpenContext: function (path, line) { ... },
//   });
(function () {
  'use strict';

  var ICON_CHAT = '<svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true"><path d="M1.75 1h8.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 10.25 10H7.061l-2.574 2.573A1.458 1.458 0 0 1 2 11.543V10h-.25A1.75 1.75 0 0 1 0 8.25v-5.5C0 1.784.784 1 1.75 1ZM1.5 2.75v5.5c0 .138.112.25.25.25h1a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h3.5a.25.25 0 0 0 .25-.25v-5.5a.25.25 0 0 0-.25-.25h-8.5a.25.25 0 0 0-.25.25Zm13 2a.25.25 0 0 0-.25-.25h-.5a.75.75 0 0 1 0-1.5h.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 14.25 12H14v1.543a1.458 1.458 0 0 1-2.487 1.03L9.22 12.28a.749.749 0 0 1 .326-1.275.749.749 0 0 1 .734.215l2.22 2.22v-2.19a.75.75 0 0 1 .75-.75h1a.25.25 0 0 0 .25-.25Z"/></svg>';
  var ICON_MINIMISE = '<svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true"><path d="M12.78 5.22a.749.749 0 0 1 0 1.06l-4.25 4.25a.749.749 0 0 1-1.06 0L3.22 6.28a.749.749 0 1 1 1.06-1.06L8 8.939l3.72-3.719a.749.749 0 0 1 1.06 0Z"/></svg>';
  var ICON_CHECK = '<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor" aria-hidden="true"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"/></svg>';

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function chronological(list) {
    return (list || []).slice().sort(function (a, b) { return String(a.asked_at).localeCompare(String(b.asked_at)); });
  }

  // pages are the open questions, oldest first.
  function pages(list) {
    return chronological(list).filter(function (d) { return d.status === 'open'; });
  }

  function openBlockingCount(list) {
    return pages(list).filter(function (d) { return d.blocking; }).length;
  }

  function summaryLine(list) {
    var n = pages(list).length;
    if (!n) return '';
    return n === 1 ? '1 decision is unanswered.' : n + ' decisions are unanswered.';
  }

  function complete(draft) {
    return !!draft && (draft.dismiss || !!draft.chosen || !!(draft.text && draft.text.trim()));
  }

  // answerText describes a draft ({chosen}|{text}|{dismiss}) or a published
  // decision (status answered or dismissed).
  function answerText(a) {
    if (!a) return '';
    if (a.dismiss || a.status === 'dismissed') return 'Dismissed';
    var text = a.text ? a.text.trim() : '';
    if (a.chosen && text) return a.chosen + ' — ' + text;
    return a.chosen || text;
  }

  function contextRef(d) {
    return d.context && d.context.path ? d.context.path + (d.context.line ? ':' + d.context.line : '') : '';
  }

  function questionPageHTML(d, draft, index, total) {
    draft = draft || {};
    var ref = contextRef(d);
    var options = (d.options || []).map(function (o, i) {
      var on = !draft.dismiss && draft.chosen === o;
      return '<button type="button" role="radio" class="decision-option" aria-checked="' + on + '" data-decision-choice="' + esc(o) + '">' +
        '<kbd>' + (i + 1) + '</kbd><span>' + esc(o) + '</span>' + (on ? ICON_CHECK : '') + '</button>';
    }).join('');
    var otherOn = !draft.dismiss && !draft.chosen && !!(draft.text && draft.text.trim());
    var other = '<label class="decision-option decision-option--other" aria-checked="' + otherOn + '">' +
      '<kbd>' + ((d.options || []).length + 1) + '</kbd>' +
      '<input type="text" data-decision-other maxlength="4000" placeholder="Other…" aria-label="Your own answer" value="' + esc(draft.chosen ? '' : draft.text || '') + '"></label>';
    var last = index === total - 1;
    return '<div class="decision-page" data-decision-page="' + index + '" data-decision-id="' + esc(d.id) + '">' +
      '<p class="decision-progress">Question ' + (index + 1) + ' of ' + total +
        (d.blocking ? '<span class="decision-blocking">Blocks approval</span>' : '') + '</p>' +
      '<p class="decision-question">' + esc(d.question) + '</p>' +
      (ref ? '<button type="button" class="decision-ref" data-decision-context="' + esc(ref) + '" title="Show in the diff">' + esc(ref) + '</button>' : '') +
      '<div class="decision-options" role="radiogroup" aria-label="Answer">' + options + other + '</div>' +
      '<footer class="decision-nav">' +
        '<button type="button" class="decision-dismiss" data-decision-dismiss aria-pressed="' + !!draft.dismiss + '">' + (draft.dismiss ? 'Will be dismissed' : 'Dismiss question') + '</button>' +
        '<button type="button" class="decision-step" data-decision-back' + (index === 0 ? ' disabled' : '') + '>Back</button>' +
        '<button type="button" class="decision-step decision-step--primary" data-decision-next>' + (last ? 'Review answers' : 'Next') + '</button>' +
      '</footer></div>';
  }

  function reviewPageHTML(open, drafts) {
    var done = open.filter(function (d) { return complete(drafts[d.id]); }).length;
    var rows = open.map(function (d, i) {
      var answer = answerText(drafts[d.id]);
      return '<li class="decision-review-row">' +
        '<p class="decision-review-question">' + esc(d.question) + '</p>' +
        '<p class="decision-review-answer' + (answer ? '' : ' decision-review-answer--empty') + '">' + esc(answer || 'No answer yet') + '</p>' +
        '<button type="button" class="decision-change" data-decision-goto="' + i + '">Change</button></li>';
    }).join('');
    return '<div class="decision-page decision-page--review" data-decision-review>' +
      '<p class="decision-progress">Your answers · ' + done + ' of ' + open.length + ' answered</p>' +
      '<ol class="decision-review">' + rows + '</ol>' +
      '<p class="decision-error" role="alert" data-decision-error hidden></p>' +
      '<footer class="decision-nav">' +
        '<button type="button" class="decision-step" data-decision-back>Back</button>' +
        '<button type="button" class="decision-step decision-step--primary" data-decision-submit' + (done ? '' : ' disabled') + '>Submit answers</button>' +
      '</footer></div>';
  }

  function answeredRows(decisions) {
    return decisions.map(function (d) {
      return '<li class="decision-review-row">' +
        '<p class="decision-review-question">' + esc(d.question) + '</p>' +
        '<p class="decision-review-answer">' + esc(answerText(d)) + '</p></li>';
    }).join('');
  }

  // historyPageHTML is the page without open questions. Right after a submit
  // it confirms the answers just sent (sentIDs) and folds earlier answers
  // away; otherwise it lists the earlier answers.
  function historyPageHTML(list, sentIDs) {
    sentIDs = sentIDs || [];
    var done = chronological(list).filter(function (d) { return d.status !== 'open'; }).reverse();
    var sent = done.filter(function (d) { return sentIDs.indexOf(d.id) >= 0; });
    var earlier = done.filter(function (d) { return sentIDs.indexOf(d.id) < 0; });
    if (!sent.length) {
      return '<div class="decision-page decision-page--history">' +
        '<h3 class="decision-section-title">Earlier answers</h3>' +
        '<ol class="decision-review">' + answeredRows(earlier) + '</ol></div>';
    }
    return '<div class="decision-page decision-page--history">' +
      '<p class="decision-sent" role="status">' + ICON_CHECK + 'Sent to the agent</p>' +
      '<ol class="decision-review">' + answeredRows(sent) + '</ol>' +
      (earlier.length ? '<details class="decision-earlier"><summary>Earlier answers <span>' + earlier.length + '</span></summary>' +
        '<ol class="decision-review">' + answeredRows(earlier) + '</ol></details>' : '') +
      '</div>';
  }

  // answersToSubmit returns the completed drafts of questions still open.
  function answersToSubmit(list, drafts) {
    return pages(list).filter(function (d) { return complete(drafts[d.id]); }).map(function (d) {
      var draft = drafts[d.id];
      var answer = draft.dismiss ? { dismiss: true } : draft.chosen ? { chosen: draft.chosen } : { text: draft.text.trim() };
      return { id: d.id, answer: answer };
    });
  }

  // dockState decides whether the dock is hidden, open or minimised: it
  // opens for an open question it has not shown before and otherwise keeps
  // the reviewer's choice.
  function dockState(list, seen, previous) {
    if (!list || !list.length) return { state: 'hidden', seen: seen || [] };
    var known = (seen || []).slice();
    var fresh = false;
    pages(list).forEach(function (d) {
      if (known.indexOf(d.id) < 0) { known.push(d.id); fresh = true; }
    });
    return { state: fresh || previous === 'open' ? 'open' : 'closed', seen: known };
  }

  function shellHTML() {
    return '<div class="decision-panel" role="region" aria-label="Questions from the agent">' +
      '<header class="decision-head"><h2 class="decision-title">Questions from the agent</h2>' +
        '<span class="decision-count" data-decision-count></span>' +
        '<button type="button" class="decision-minimise" data-decision-minimise aria-label="Minimise questions">' + ICON_MINIMISE + '</button></header>' +
      '<div class="decision-body" data-decision-body></div></div>' +
      '<button type="button" class="decision-launcher" data-decision-launcher aria-expanded="false">' + ICON_CHAT +
        '<span>Agent asks</span><span class="decision-badge" data-decision-badge></span></button>';
  }

  function setState(root, state) {
    root.dataset.state = state;
    root.hidden = state === 'hidden';
    var launcher = root.querySelector('[data-decision-launcher]');
    if (launcher) launcher.setAttribute('aria-expanded', state === 'open' ? 'true' : 'false');
  }

  // draw renders the current view: a question page, the review page, or the
  // history of past answers.
  function draw(root, focus) {
    var s = root._decisionState;
    var open = pages(s.list);
    var body = root.querySelector('[data-decision-body]');
    if (!open.length) {
      s.view = 'history';
      body.innerHTML = historyPageHTML(s.list, s.sent);
    } else if (s.view === 'review') {
      body.innerHTML = reviewPageHTML(open, s.drafts);
    } else {
      s.view = 'question';
      s.index = Math.max(0, Math.min(s.index, open.length - 1));
      body.innerHTML = questionPageHTML(open[s.index], s.drafts[open[s.index].id], s.index, open.length);
    }
    root.querySelector('[data-decision-count]').textContent = open.length ? open.length + ' open' : 'All answered';
    var badge = root.querySelector('[data-decision-badge]');
    badge.textContent = open.length ? String(open.length) : '';
    badge.hidden = !open.length;
    root.classList.toggle('decision-dock--blocking', openBlockingCount(s.list) > 0);
    if (focus && root.dataset.state === 'open') {
      var target = body.querySelector('[aria-checked="true"]') || body.querySelector('.decision-option, [data-decision-submit]:not([disabled]), [data-decision-back]');
      if (target) (target.querySelector('input') || target).focus({ preventScroll: true });
    }
  }

  function go(root, view, index) {
    var s = root._decisionState;
    s.view = view;
    if (index !== undefined) s.index = index;
    draw(root, true);
  }

  function advance(root) {
    var s = root._decisionState;
    if (s.index < pages(s.list).length - 1) go(root, 'question', s.index + 1);
    else go(root, 'review');
  }

  function currentDraft(root) {
    var s = root._decisionState;
    var d = pages(s.list)[s.index];
    if (!d) return null;
    s.drafts[d.id] = s.drafts[d.id] || {};
    return s.drafts[d.id];
  }

  function choose(root, value) {
    var draft = currentDraft(root);
    if (!draft) return;
    draft.chosen = value;
    draft.text = '';
    draft.dismiss = false;
    advance(root);
  }

  function submit(root, button) {
    var s = root._decisionState;
    var answers = answersToSubmit(s.list, s.drafts);
    if (!answers.length) return;
    var error = root.querySelector('[data-decision-error]');
    button.disabled = true;
    button.textContent = 'Submitting…';
    Promise.resolve(root._decisionHandlers.onSubmit(answers)).then(function () {
      answers.forEach(function (a) { delete s.drafts[a.id]; });
      s.sent = answers.map(function (a) { return a.id; });
      s.view = 'question';
      s.index = 0;
      draw(root, false);
      scheduleMinimise(root);
    }).catch(function (err) {
      button.disabled = false;
      button.textContent = 'Submit answers';
      if (error) {
        error.hidden = false;
        error.textContent = 'Could not submit: ' + (err && err.message ? err.message : 'try again');
      }
    });
  }

  // Once every question is answered, the dock gets out of the way: it
  // minimises a few seconds after the submit unless the reviewer is using it.
  var MINIMISE_AFTER_SUBMIT_MS = 4000;
  function scheduleMinimise(root) {
    clearTimeout(root._decisionMinimise);
    root._decisionMinimise = setTimeout(function () {
      var panel = root.querySelector('.decision-panel');
      var busy = panel.contains(document.activeElement) && document.activeElement !== root.querySelector('[data-decision-submit]') || panel.matches(':hover');
      if (root.dataset.state !== 'open' || pages(root._decisionState.list).length) return;
      if (busy) scheduleMinimise(root); // try again once the reviewer moves on
      else setState(root, 'closed');
    }, MINIMISE_AFTER_SUBMIT_MS);
  }

  function onClick(root, event) {
    var target = event.target.closest('button');
    if (!target) return;
    var s = root._decisionState;
    if (target.hasAttribute('data-decision-launcher')) {
      setState(root, root.dataset.state === 'open' ? 'closed' : 'open');
      if (root.dataset.state === 'open') draw(root, true);
    } else if (target.hasAttribute('data-decision-minimise')) {
      setState(root, 'closed');
      root.querySelector('[data-decision-launcher]').focus();
    } else if (target.hasAttribute('data-decision-choice')) {
      choose(root, target.getAttribute('data-decision-choice'));
    } else if (target.hasAttribute('data-decision-dismiss')) {
      var draft = currentDraft(root);
      if (draft.dismiss) { draft.dismiss = false; draw(root, false); } else { draft.dismiss = true; draft.chosen = ''; draft.text = ''; advance(root); }
    } else if (target.hasAttribute('data-decision-next')) {
      advance(root);
    } else if (target.hasAttribute('data-decision-back')) {
      if (s.view === 'review') go(root, 'question', pages(s.list).length - 1);
      else go(root, 'question', s.index - 1);
    } else if (target.hasAttribute('data-decision-goto')) {
      go(root, 'question', Number(target.getAttribute('data-decision-goto')));
    } else if (target.hasAttribute('data-decision-submit')) {
      submit(root, target);
    } else if (target.hasAttribute('data-decision-context')) {
      var ref = target.getAttribute('data-decision-context');
      var at = ref.lastIndexOf(':');
      root._decisionHandlers.onOpenContext(at > 0 ? ref.slice(0, at) : ref, at > 0 ? Number(ref.slice(at + 1)) : 0);
    }
  }

  function onKey(root, event) {
    var s = root._decisionState;
    if (event.key === 'Escape') {
      event.stopPropagation();
      setState(root, 'closed');
      root.querySelector('[data-decision-launcher]').focus();
      return;
    }
    var inOther = event.target.hasAttribute && event.target.hasAttribute('data-decision-other');
    if (inOther && event.key === 'Enter' && !event.isComposing) {
      event.preventDefault();
      advance(root);
      return;
    }
    if (inOther || s.view !== 'question' || event.metaKey || event.ctrlKey || event.altKey) return;
    var d = pages(s.list)[s.index];
    var n = Number(event.key);
    if (d && n >= 1 && n <= (d.options || []).length) {
      event.preventDefault();
      choose(root, d.options[n - 1]);
    } else if (d && n === (d.options || []).length + 1) {
      event.preventDefault();
      root.querySelector('[data-decision-other]').focus();
    }
  }

  function bind(root) {
    root.addEventListener('click', function (event) { onClick(root, event); });
    root.addEventListener('keydown', function (event) {
      if (root.dataset.state === 'open' && root.querySelector('.decision-panel').contains(event.target)) onKey(root, event);
    });
    root.addEventListener('input', function (event) {
      if (!event.target.hasAttribute('data-decision-other')) return;
      var draft = currentDraft(root);
      draft.text = event.target.value;
      draft.chosen = '';
      draft.dismiss = false;
      var row = event.target.closest('.decision-option');
      row.setAttribute('aria-checked', String(!!draft.text.trim()));
      root.querySelectorAll('[data-decision-choice]').forEach(function (b) { b.setAttribute('aria-checked', 'false'); var icon = b.querySelector('svg'); if (icon) icon.remove(); });
    });
  }

  // render draws the dock into root (a fixed element in index.html). Drafts
  // survive re-renders while their question stays open.
  function render(root, list, handlers) {
    root._decisionHandlers = handlers;
    if (!root.dataset.decisionsBound) {
      root.innerHTML = shellHTML();
      root.dataset.decisionsBound = '1';
      root._decisionState = { list: [], drafts: {}, view: 'question', index: 0, seen: [], sent: [] };
      bind(root);
    }
    var s = root._decisionState;
    s.list = list || [];
    var openIDs = pages(s.list).map(function (d) { return d.id; });
    if (openIDs.length) s.sent = [];
    Object.keys(s.drafts).forEach(function (id) { if (openIDs.indexOf(id) < 0) delete s.drafts[id]; });
    var editing = root.contains(document.activeElement) && document.activeElement.hasAttribute('data-decision-other');
    var next = dockState(s.list, s.seen, root.dataset.state);
    var opening = next.state === 'open' && root.dataset.state !== 'open';
    s.seen = next.seen;
    setState(root, next.state);
    if (!editing) draw(root, opening);
  }

  var api = {
    openBlockingCount: openBlockingCount, summaryLine: summaryLine, pages: pages, answerText: answerText,
    questionPageHTML: questionPageHTML, reviewPageHTML: reviewPageHTML, historyPageHTML: historyPageHTML,
    answersToSubmit: answersToSubmit, dockState: dockState, render: render,
  };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.decisions = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
