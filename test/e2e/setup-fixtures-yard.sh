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

# Isolate from the user's config and preference store; the fake home also
# sets HUNKYARD_PREFERENCES_HOME, so the Preferences tab reads only this
# seeded store.
e2e_export_fake_home "$DIR"

REPO="$DIR/repo"
mkdir -p "$REPO"
cd "$REPO"
git init -q -b main
git config user.email "test@test.com"
git config user.name "Test"
printf 'package a\n' > a.go
git add a.go
git commit -qm base
printf 'package a\n\nvar X = 1\n' > a.go

if [ -z "${HUNKYARD_BIN:-}" ]; then
  HUNKYARD_BIN="$BIN_DIR/$(e2e_bin_name)"
  (cd "$HUNKYARD_SRC" && go build -o "$HUNKYARD_BIN" ./cmd/hunkyard)
fi


echo "we dont use single line method definitions" | "$HUNKYARD_BIN" preferences capture --explicit \
  --directive "Write method definitions over multiple lines." --scope global --surface ruby --json >/dev/null
echo "keep the tests local" | "$HUNKYARD_BIN" preferences capture --explicit \
  --directive "Keep test files out of commits." --scope repository --repo-key orion-a1b2c3d4e5f6 --surface testing --json >/dev/null
# One captured observation still waiting for maintenance.
echo "maybe prefer early returns in handlers" | "$HUNKYARD_BIN" preferences capture --json >/dev/null

# A small review whose Settings carry the Preferences tab.
e2e_serve "$HUNKYARD_BIN" _serve --no-open --port "$PORT"
