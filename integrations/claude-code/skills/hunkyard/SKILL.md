---
name: hunkyard
description: Use when the user asks for Hunkyard browser review of local changes, a commit range, or files in Claude CLI. A generic review request does not start an interactive Hunkyard review.
---

# Hunkyard in Claude CLI

Read [the shared review protocol](references/review-workflow.md), then use the current Claude session to launch and process the review.

For code-change reviews, preserve Git diff mode when selecting code paths. Follow the shared protocol’s mode check so a source-file review is not presented as a Git diff.

Prefix **every** Hunkyard command with `HUNKYARD_OWNER_ID="claude:${CLAUDE_SESSION_ID}"`. Claude's local skill substitution supplies this session's ID; keep that owner for launch, waits, learning, acknowledgment, and replies. If the placeholder is unavailable, use the actual stable current session ID consistently. Never change owners on reconnect or launch a second Claude process.

Example: `HUNKYARD_OWNER_ID="claude:${CLAUDE_SESSION_ID}" hunkyard --range HEAD~1..HEAD`.

Use Bash with `run_in_background: true` for the initial review. If it returns `status: pending`, start `hunkyard feedback await --session ID --follow --json` once in the background with the same owner. Wait on that same task using TaskOutput or its completion notification; do not run sleep loops, restart the review, or repeatedly issue bounded waits. The pending process returns as soon as feedback is published. Relay the actual printed local URL while the user reviews.

When installed, the Claude `Stop` hook uses `asyncRewake: true` and exits 2 on publication to wake an idle session. Process the named review with the same persistent await command. Hook delivery is a notification, not acknowledgment; use the shared protocol to learn, reply, acknowledge, and advance the round.

Apply returned selected skills through the existing Claude session's normal trust and permissions, then classify human revisions, reply, and acknowledge as described in the shared protocol. Only explicit approval ends the loop. Hunkyard itself performs no provider requests or console injection.
