(function (root) {
  'use strict';
  function selection(container) {
    return Array.from(container.querySelectorAll('input[name="review-skill"]:checked')).map(function (input) { return input.value; });
  }
  async function render(container, fetcher) {
    var response = await fetcher('/api/skills');
    if (!response.ok) throw new Error('Cannot load the launching host’s skills');
    var skills = (await response.json()).skills;
    var selected = new Set(selection(container));
    var fieldset = document.createElement('fieldset');
    fieldset.className = 'publish-skills';
    var legend = document.createElement('legend'); legend.textContent = 'Skills to apply'; fieldset.appendChild(legend);
    if (!skills.length) {
      var empty = document.createElement('p'); empty.textContent = 'No skills available for this review.'; fieldset.appendChild(empty);
    }
    var options = document.createElement('div');
    options.className = 'publish-skills-options';
    skills.forEach(function (skill) {
      var label = document.createElement('label');
      var input = document.createElement('input'); input.type = 'checkbox'; input.name = 'review-skill'; input.value = skill.id; input.checked = selected.has(skill.id);
      label.appendChild(input); label.appendChild(document.createTextNode(' ' + skill.name));
      if (skill.description) label.title = skill.description;
      options.appendChild(label);
    });
    if (skills.length) fieldset.appendChild(options);
    container.replaceChildren(fieldset);
  }
  var api = { selection: selection, render: render };
  if (typeof module === 'object') module.exports = api;
  else root.hunkyardSkills = api;
}(typeof window === 'object' ? window : globalThis));
