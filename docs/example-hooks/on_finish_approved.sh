#!/usr/bin/env sh
# Example on_finish_approved hook — append an audit line per approved review.
#
# Hunkyard exposes the review context as env vars (HUNKYARD_*) and a JSON payload on
# stdin. See docs/agent-hooks.md for the full reference. Runs with $PWD = repo
# root. Edit freely — this file is an example, not a default behavior.

: "${HUNKYARD_REVIEW_PATH:?missing HUNKYARD_REVIEW_PATH}"
: "${HUNKYARD_SESSION_KEY:=unknown}"
: "${HUNKYARD_MODE:=}"

log="$HOME/.hunkyard/reviews/approved.log"
mkdir -p "$(dirname "$log")"

ts=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date 2>/dev/null || echo "?")
printf '%s\t%s\t%s\t%s\n' "$ts" "$HUNKYARD_SESSION_KEY" "$HUNKYARD_MODE" "$HUNKYARD_REVIEW_PATH" >>"$log"

echo "hunkyard: logged approval to $log" >&2