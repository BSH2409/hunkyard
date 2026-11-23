(function (root) {
  'use strict';

  var recommendations = [
    { name: 'Superpowers', key: 'superpowers' },
    { name: 'Naming Analyzer', key: 'naminganalyzer' },
    { name: 'Hunkyard Preferences', key: 'hunkyardpreferences' },
    { name: 'Impeccable', key: 'impeccable' },
  ];

  function selection(container) {
    return Array.from(container.querySelectorAll('input[name="review-skill"]:checked:not(:disabled)')).map(function (input) { return input.value; });
  }

  async function render(container, fetcher) {
    var response = await fetcher('/api/skills');
    if (!response.ok) throw new Error('Cannot load the launching host’s skills');
    var skills = (await response.json()).skills;
    var selected = new Set(selection(container));
    var fieldset = document.createElement('fieldset');
    fieldset.className = 'publish-skills';
    var legend = document.createElement('legend'); legend.textContent = 'Skills to apply'; fieldset.appendChild(legend);
    var options = document.createElement('div');
    options.className = 'publish-skills-options';
    skills.forEach(function (skill) {
      var label = document.createElement('label');
      var input = document.createElement('input'); input.type = 'checkbox'; input.name = 'review-skill'; input.value = skill.id; input.checked = selected.has(skill.id);
      label.appendChild(input); label.appendChild(document.createTextNode(' ' + skill.name));
      if (skill.description) label.title = skill.description;
      options.appendChild(label);
    });
    var missing = recommendations.filter(function (recommended) {
      return !skills.some(function (skill) {
        var id = skill.id.toLowerCase().split(':').pop().replace(/[^a-z0-9]/g, '');
        var name = skill.name.toLowerCase().split(':').pop().replace(/[^a-z0-9]/g, '');
        return id === recommended.key || name === recommended.key ||
          (recommended.key === 'superpowers' && (skill.id.toLowerCase().startsWith('superpowers:') || skill.name.toLowerCase().startsWith('superpowers:')));
      });
    });
    missing.forEach(function (skill) {
      var label = document.createElement('label');
      label.className = 'publish-skill-unavailable';
      var input = document.createElement('input'); input.type = 'checkbox'; input.disabled = true;
      label.appendChild(input);
      var text = document.createElement('span'); text.textContent = skill.name; label.appendChild(text);
      var badge = document.createElement('span'); badge.className = 'publish-skill-recommended'; badge.textContent = '*'; badge.setAttribute('aria-label', 'Recommended'); label.appendChild(badge);
      options.appendChild(label);
    });
    fieldset.appendChild(options);
    if (missing.length) {
      var hint = document.createElement('p');
      hint.textContent = '* Recommended. Unavailable skills must be offered by the launching agent.';
      fieldset.appendChild(hint);
    }
    container.replaceChildren(fieldset);
  }
  var api = { selection: selection, render: render };
  if (typeof module === 'object') module.exports = api;
  else root.hunkyardSkills = api;
}(typeof window === 'object' ? window : globalThis));
