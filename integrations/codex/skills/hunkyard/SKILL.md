---
name: hunkyard
description: Use when the user asks for Hunkyard browser review of local changes, a commit range, or files in Codex. A generic review request does not start an interactive Hunkyard review.
---

# Hunkyard in Codex

Use the current Codex session to launch the local browser review and process its published feedback. Read [the shared review protocol](references/review-workflow.md), then follow its persistent wait, skill application, preference classification, reply, and acknowledgment loop.

For code-change reviews, preserve Git diff mode when selecting code paths. Follow the shared protocol’s mode check so a source-file review is not presented as a Git diff.

Codex normally exports `CODEX_THREAD_ID`; Hunkyard derives its owner from that value. If it is absent, set `HUNKYARD_OWNER_ID="codex:<this session's stable ID>"` on every Hunkyard command. Reuse the same owner across waits and tool calls. Never invent a new owner on reconnect or resume another agent session.

Run commands with the native execution tool. The initial review command may return `pending` after 45 seconds. On `pending`, launch `hunkyard feedback await --session ID --follow --json` once, keeping the same owner environment. Use the tool's persistent/background execution support and allow time for the human review. If it yields a running tool/session ID, wait on that same tool session until completion. Hunkyard renews pending HTTP waits internally and prints only a structured publication; the process stays open until publication, cancellation, or an error. After an interrupted or failed waiter, reconnect with the same `feedback await --follow` command. Share the printed local URL while the user reviews.

Hunkyard does not call a provider, inject terminal input, or start another Codex process. Apply returned selected skills in this existing session, using its normal trust and permissions. Keep the review open until an explicit approval has been processed and acknowledged.
