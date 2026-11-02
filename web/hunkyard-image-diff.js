// Image diff for changed PNG, JPEG, GIF and WebP files: base and head side by
// side, a slider that reveals the head over the base, or an onion skin that
// fades between them. Bytes come from /api/file/image; app.js mounts the
// element in the file's Pierre item.
(function () {
  'use strict';

  var IMAGE_PATH = /\.(png|jpe?g|gif|webp)$/i;
  var MODES = [['side', 'Side by side'], ['slider', 'Slider'], ['onion', 'Onion skin']];

  function isImagePath(path) { return IMAGE_PATH.test(path || ''); }

  function make(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text) node.textContent = text;
    return node;
  }

  function pane(label, url, sideClass, onError) {
    var figure = make('figure', 'image-diff-pane ' + sideClass);
    var img = make('img');
    img.setAttribute('src', url);
    img.setAttribute('alt', label);
    img.setAttribute('loading', 'lazy');
    var caption = make('figcaption', 'image-diff-caption', label);
    img.addEventListener('load', function () {
      caption.textContent = label + ' · ' + img.naturalWidth + '×' + img.naturalHeight;
    });
    img.addEventListener('error', onError);
    figure.appendChild(img);
    figure.appendChild(caption);
    return { figure: figure, img: img };
  }

  // build returns the image diff element. opts: { path, status, baseUrl,
  // headUrl, onFallback } — onFallback runs once when an image cannot load.
  function build(opts) {
    var root = make('div', 'image-diff');
    var failed = false;
    function fail() {
      if (failed) return;
      failed = true;
      if (opts.onFallback) opts.onFallback();
    }
    var single = opts.status === 'added' || opts.status === 'untracked' ? ['Added', opts.headUrl]
      : opts.status === 'deleted' ? ['Deleted', opts.baseUrl] : null;
    if (single) {
      root.dataset.mode = 'single';
      var stage = make('div', 'image-diff-stage');
      stage.appendChild(pane(single[0], single[1], 'image-diff-pane--only', fail).figure);
      root.appendChild(stage);
      return root;
    }
    root.dataset.mode = 'side';
    var toolbar = make('div', 'image-diff-toolbar file-header-toggle');
    toolbar.setAttribute('role', 'group');
    toolbar.setAttribute('aria-label', 'Image comparison');
    var stageEl = make('div', 'image-diff-stage');
    var base = pane('Before', opts.baseUrl, 'image-diff-pane--base', fail);
    var head = pane('After', opts.headUrl, 'image-diff-pane--head', fail);
    stageEl.appendChild(base.figure);
    stageEl.appendChild(head.figure);
    var range = make('input', 'image-diff-range');
    range.setAttribute('type', 'range');
    range.setAttribute('min', '0');
    range.setAttribute('max', '100');
    range.setAttribute('value', '50');
    range.setAttribute('aria-label', 'Compare');
    range.hidden = true;
    function applyRange() {
      var value = Number(range.value);
      if (root.dataset.mode === 'slider') {
        head.figure.style.clipPath = 'inset(0 0 0 ' + value + '%)';
        head.figure.style.opacity = '';
      } else if (root.dataset.mode === 'onion') {
        head.figure.style.opacity = String(value / 100);
        head.figure.style.clipPath = '';
      } else {
        head.figure.style.opacity = '';
        head.figure.style.clipPath = '';
      }
    }
    range.addEventListener('input', applyRange);
    var buttons = [];
    MODES.forEach(function (mode) {
      var btn = make('button', 'toggle-btn', mode[1]);
      btn.setAttribute('type', 'button');
      btn.setAttribute('aria-pressed', mode[0] === 'side' ? 'true' : 'false');
      btn.addEventListener('click', function () {
        root.dataset.mode = mode[0];
        buttons.forEach(function (b, i) {
          var on = MODES[i][0] === mode[0];
          b.setAttribute('aria-pressed', on ? 'true' : 'false');
          b.classList.toggle('active', on);
        });
        range.hidden = mode[0] === 'side';
        applyRange();
      });
      if (mode[0] === 'side') btn.classList.add('active');
      buttons.push(btn);
      toolbar.appendChild(btn);
    });
    root.appendChild(toolbar);
    root.appendChild(stageEl);
    root.appendChild(range);
    return root;
  }

  var api = { isImagePath: isImagePath, build: build };
  if (typeof window !== 'undefined') {
    window.hunkyard = window.hunkyard || {};
    window.hunkyard.imageDiff = api;
  }
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
})();
