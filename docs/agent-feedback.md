# Agent feedback delivery

Hunkyard keeps review processing in the Claude or Codex session that launched it. The browser publishes durable feedback batches; the CLI receives them through an authenticated local endpoint. The launching agent owns implementation feedback. Explicitly invited group-discussion participants use separate read-only provider CLI calls as described below.

## Launch and verify the comparison

For ongoing edits, launch a working-tree review:

```sh
hunkyard
```

Use an explicit range when reviewing a fixed commit comparison:

```sh
hunkyard --range HEAD~1..HEAD
```

A range is pinned to the resolved commits. Staged and unstaged edits are outside that comparison. The browser shows a **Review working tree** action to switch explicitly. Refresh in the working tree includes current edits and clears browser commit/scope filters; it does not start another review round. File reviews reread current content and recompute available code diffs.

Check `/api/session` before announcing the review: changed code should use `mode: git`, and ongoing fixes should use `focus.kind: working_tree`. Markdown-only, unchanged-file, and non-Git reviews can use file mode.

## Keep one waiter

The initial command can return `status: pending`. Start one persistent waiter using the returned session ID:

```sh
hunkyard feedback await --session ID --follow --json
```

Use the host's background/native execution tool and wait on that same task. Do not create sleep loops or relaunch the review on each timeout. Hunkyard renews bounded HTTP waits silently, checks the durable queue on each request, and releases an active wait on publication. A lost notification is recovered by the next queue check.

Claude commands must retain `HUNKYARD_OWNER_ID="claude:${CLAUDE_SESSION_ID}"`. Codex normally uses `CODEX_THREAD_ID`; when unavailable, provide the same explicit owner ID on every command. Credentials remain in private files and authenticated headers. The CLI verifies the daemon's challenge response before sending them.

A paused review is not approval. Resume it with `hunkyard --session ID`. An interrupted waiter reconnects using `feedback await --follow`, without advancing the round.

## Claude wake hook

Install the host integration explicitly:

```sh
hunkyard preferences install-hook --host claude --json
```

Inspect the installed command in Claude's `/hooks` UI and follow the host's trust flow. Do not synthesize a trust decision.

The `Stop` wake handler uses `asyncRewake: true`, waits on the authenticated publication endpoint, and exits with code 2 when feedback is available. The host session ID comes from the hook payload. A newer wake token supersedes older handlers; claiming a notification also checks the token under the session lock. Notification claims do not acknowledge feedback.

Normal asynchronous hook output waits for a later conversation turn. Claude's documented `asyncRewake` mechanism can wake an idle interactive session. Its timeout is enforced, and noninteractive session teardown can cancel background work. Keep the persistent waiter as the delivery path; SessionStart and Stop notices also identify queued feedback.

See the [Claude hooks reference](https://code.claude.com/docs/en/hooks#run-hooks-in-the-background). An expired Claude login must be renewed with `claude auth login` before testing the live host.

## Codex delivery

Use the persistent native execution task. Asynchronous Codex hook output is delivered at a safe point; do not assume that it starts an idle turn.

An integration that owns a Codex app-server connection can use `turn/start` for a new turn and `turn/steer` for an active turn. Hunkyard's CLI integration does not own that connection or launch another Codex process to process feedback.

See the official [Codex hooks](https://learn.chatgpt.com/docs/hooks#run-hooks-in-the-background) and [app-server](https://learn.chatgpt.com/docs/app-server) documentation.

## Process each publication

1. Read the explicit disposition, snapshot identity, comments, decisions, and selected skills. A timeout or empty comment list is not approval.
2. Classify each human revision with `hunkyard feedback learn`. Keep blocked or failed classifications pending.
3. Apply the requested changes, verify them, and reply to the relevant threads. Leave resolution to the reviewer unless requested.
4. Acknowledge the handled batch with `hunkyard feedback ack --session ID --batch BATCH --json`.
5. For `comment` or `request_changes`, run `hunkyard --session ID` once to start the next round, then wait again. Only explicit approval ends the loop.

The shared [review protocol](../integrations/shared/review-workflow.md) defines the full JSON and command contract.

## Review conversation

The bottom-right **Review conversation** panel shares the review's existing threads and agent replies. Closing it keeps an unsent draft. Send delivers the message to the selected room participants; **Review changes** opens the existing publication flow, including disposition and skill selection.

Use repository-relative references such as `internal/server/server.go:42` or `web/app.js#L120` in comments and replies. References to reviewed files are clickable and keyboard accessible; they navigate through the virtualized file list. Unreviewed paths remain plain text.

## Group review room

The bottom-right chat launcher opens a separate discussion surface over the existing review conversation. Your messages appear on the right; agent replies appear on the left with their names and an Agent badge. The launcher counts unread agent messages. The upper-left grip resizes the panel while its right edge stays anchored; arrow keys also resize it. The composer stays below the scrolling transcript and names the destination before sending. Enter sends a message and Shift+Enter adds a line.

Use the visible **Claude** and **Codex** buttons to enable either or both for this review. Selected participants have a check mark; click again to remove them. Extra participants are disabled by default. Invitations are saved with the review and do not run model probes. Sending with invited participants uses their installed, authenticated CLI accounts and can incur provider usage. `@Claude` or `@Codex` addresses one invited participant; otherwise both invited agents receive the question. Without invited participants, Send publishes a comment batch to the launching coding agent through the existing feedback queue.

For a nondefault CLI profile, set `CLAUDE_CONFIG_DIR` for Claude or `CODEX_HOME` for Codex before launching Hunkyard. Participant calls use those same authentication directories.

The built-in discussion adapters follow Nerd Townhall's read-only, frozen-evidence and explicit-failure approach. Each request includes the reviewer's question, recent review conversation, and bounded code from the current review snapshot. Sensitive file names and recognized credential values are omitted or redacted; omissions are reported in the context packet. Agent replies can link to reviewed code using `path:line` references. Claude uses Haiku with low effort; Codex uses `gpt-6.1-sol` with medium effort. Neither adapter is allowed to edit reviewed files, and no participant silently replaces another.

The UI shows each participant thinking, replying, or failing. Authentication, quota, rate limit, timeout and missing-CLI failures have distinct states. A failed participant does not prevent another from answering. Retry is explicit and creates a new provider request; there is no automatic paid recovery loop. Each call is limited to three minutes, and a room admits at most four active calls.

Discussion requests and participant settings are stored privately in `discussion.json` alongside the review. A request ID prevents duplicate submissions from launching duplicate calls. Agent replies keep their machine provenance in the normal review file. Requests interrupted by a daemon restart remain interrupted until retried. **Review changes** opens the existing disposition/approval flow; an advisory chat reply is not approval.
