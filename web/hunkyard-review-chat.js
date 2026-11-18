(function(root) {
  'use strict';

  function transcript(comments) {
    const messages = [];
    comments.forEach(function(comment) {
      messages.push({ key: comment.id, thread: comment, message: comment, reply: false });
      (comment.replies || []).forEach(function(reply) {
        messages.push({ key: comment.id + '/' + reply.id, thread: comment, message: reply, reply: true });
      });
    });
    return messages.sort(function(a, b) { return (Date.parse(a.message.created_at) || 0) - (Date.parse(b.message.created_at) || 0); });
  }

  function isAgent(message) {
    if (message.author_kind) return message.author_kind === 'agent';
    return /^(claude|codex)(?:\b|[-_])/i.test(message.author || '');
  }

  function recipients(body, participants) {
    const mentions = ['claude', 'codex'].filter(function(agent) { return new RegExp('@' + agent + '\\b', 'i').test(body); });
    return mentions.length ? mentions : participants.slice();
  }

  function latestResponses(requests) {
    const latest = new Map();
    requests.forEach(function(request) {
      (request.responses || []).forEach(function(response) {
        latest.set(request.comment_id + '/' + (request.message_id || '') + '/' + response.agent, { request: request, response: response });
      });
    });
    return Array.from(latest.values());
  }

  const api = { transcript: transcript, isAgent: isAgent, recipients: recipients, latestResponses: latestResponses };
  if (typeof module === 'object' && module.exports) module.exports = api;
  root.hunkyard = root.hunkyard || {};
  root.hunkyard.reviewChat = api;
})(typeof window === 'undefined' ? globalThis : window);
