---
name: hunkyard-cli
description: Use when reading local Hunkyard feedback, posting replies, or integrating a plugin with its owner-bound CLI/JSON protocol. Not for invoking an interactive review loop — that's the `hunkyard` skill.
user-invocable: false
---

# Local Hunkyard CLI

Read [the shared review protocol](../hunkyard/references/review-workflow.md). Prefix every command with `HUNKYARD_OWNER_ID="claude:${CLAUDE_SESSION_ID}"` and target `--session ID` from the launching review.

- `hunkyard feedback await --session ID --timeout 45s --json`
- `hunkyard feedback learn --session ID --batch BATCH --decisions FILE --json`
- `hunkyard feedback ack --session ID --batch BATCH --json`
- `hunkyard comments --session ID --json --all`
- `hunkyard comment --session ID --reply-to COMMENT --author Claude 'What changed'`

Use `--json --file FILE` for bulk/multiline replies. The review-summary pseudo-comment is answered as a review-level comment. Let the human resolve threads. Published human revisions are classified through the Go preference bridge before acknowledgment. Hunkyard makes no remote requests; the existing Claude session owns its model-provider communication.
