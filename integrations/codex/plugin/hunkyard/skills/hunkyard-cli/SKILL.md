---
name: hunkyard-cli
description: Use when reading local Hunkyard feedback, posting replies, or integrating a plugin with its owner-bound CLI/JSON protocol. Not for invoking an interactive review loop — that's the `hunkyard` skill.
---

# Local Hunkyard CLI

Read [the shared review protocol](../hunkyard/references/review-workflow.md) for publication, selected skills, learn-preferences classification, acknowledgment, and next rounds. Use the same stable owner environment as the launch; target the explicit `--session ID`.

- `hunkyard feedback await --session ID --follow --json`
- `hunkyard feedback learn --session ID --batch BATCH --decisions FILE --json`
- `hunkyard feedback ack --session ID --batch BATCH --json`
- `hunkyard comments --session ID --json --all`
- `hunkyard comment --session ID --reply-to COMMENT --author Codex 'What changed'`

`--follow` keeps one process waiting silently until publication, cancellation, or an error. Wait on the same native tool session until it completes. Omit `--follow` for a bounded one-shot wait; `--timeout` (default 45s) bounds each HTTP request.

Use `--json --file FILE` for bulk/multiline replies. The review-summary pseudo-comment is answered as a review-level comment. Let the human resolve threads. Hunkyard has no remote PR sync, hosted sharing, provider calls, or update requests. Published feedback stays local until this launching host processes it.
