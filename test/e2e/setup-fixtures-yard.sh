#!/usr/bin/env bash
set -euo pipefail

PORT="${1:-3137}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HUNKYARD_SRC="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"
DIR=$(e2e_native_tempdir)
BIN_DIR=$(e2e_native_tempdir)
e2e_cleanup_install
e2e_cleanup_add "$DIR" "$BIN_DIR"

cd "$DIR"

if [ -z "${HUNKYARD_BIN:-}" ]; then
  HUNKYARD_BIN="$BIN_DIR/$(e2e_bin_name)"
  (cd "$HUNKYARD_SRC" && go build -o "$HUNKYARD_BIN" ./cmd/hunkyard)
fi

# Isolate from the user's config and preference store; the fake home also
# sets LEARN_PREFERENCES_HOME, so the Yard reads only this seeded store.
e2e_export_fake_home "$DIR"

echo "we dont use single line method definitions" | "$HUNKYARD_BIN" preferences capture --explicit \
  --directive "Write method definitions over multiple lines." --scope global --surface ruby --json >/dev/null
echo "keep the tests local" | "$HUNKYARD_BIN" preferences capture --explicit \
  --directive "Keep test files out of commits." --scope repository --repo-key orion-a1b2c3d4e5f6 --surface testing --json >/dev/null

# The standalone Preference Yard: no review session.
e2e_serve "$HUNKYARD_BIN" yard --no-open --port "$PORT"
