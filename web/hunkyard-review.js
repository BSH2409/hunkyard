(function () {
  'use strict';
  var dialog = document.getElementById('publishReviewDialog');
  if (!dialog) return;
  var form = dialog.querySelector('form');
  var summary = document.getElementById('publishSummary');
  var submit = document.getElementById('publishReviewSubmit');
  var pending = null;
  var request = null;
  var fingerprint = '';
  window.hunkyardReview = {
    prepare: function (publicationKey) {
      if (pending) return pending;
      pending = (async function () {
      if (window.hunkyardSkills) await window.hunkyardSkills.render(document.getElementById('publishSkills'), window.fetch.bind(window));
      return new Promise(function (resolve) {
        var previousFocus = document.activeElement;
        function close(value) {
          form.removeEventListener('submit', publish);
          dialog.removeEventListener('cancel', cancel);
          document.getElementById('publishReviewCancel').removeEventListener('click', cancel);
          dialog.close();
          pending = null;
          if (previousFocus) previousFocus.focus();
          resolve(value);
        }
        function cancel(event) { event.preventDefault(); close(false); }
        function publish(event) {
          event.preventDefault();
          var value = { disposition: new FormData(form).get('disposition'), summary: summary.value, skill_ids: window.hunkyardSkills ? window.hunkyardSkills.selection(document.getElementById('publishSkills')) : [] };
          var context = typeof publicationKey === 'function' ? publicationKey() : null;
          if (context && context.displayed_files) value.displayed_files = context.displayed_files;
          var nextFingerprint = JSON.stringify([value, context]);
          if (!request || fingerprint !== nextFingerprint) {
            request = crypto.randomUUID();
            fingerprint = nextFingerprint;
          }
          value.request_id = request;
          close(value);
        }
        form.addEventListener('submit', publish);
        dialog.addEventListener('cancel', cancel);
        document.getElementById('publishReviewCancel').addEventListener('click', cancel);
        submit.disabled = false;
        dialog.showModal();
        summary.focus();
      });
      }()).catch(function (error) { pending = null; throw error; });
      return pending;
    },
    published: function () { request = null; fingerprint = ''; summary.value = ''; },
  };
}());
