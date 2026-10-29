#!/usr/bin/env bash
# End-to-end integration test runner for the hunkyard ↔ GitHub PR roundtrip.
# Builds hunkyard, then runs Go tests behind build tag e2e_github against a real
# GitHub PR in the sandbox repo configured via HUNKYARD_ROUNDTRIP_REPO.
#
# Usage:
#   ./scripts/e2e-roundtrip.sh                 # all scenarios
#   ./scripts/e2e-roundtrip.sh -run TestX      # one scenario
#   ./scripts/e2e-roundtrip.sh -v -count=1     # extra go test args
set -euo pipefail

HUNKYARD_DIR="$(cd "$(dirname "$0")/.." && pwd)"

if command -v /opt/homebrew/bin/mise >/dev/null 2>&1; then
  eval "$(/opt/homebrew/bin/mise env -s bash -C "$HUNKYARD_DIR" 2>/dev/null)" || true
fi

if [ -z "${HUNKYARD_ROUNDTRIP_REPO:-}" ]; then
  echo "✗ HUNKYARD_ROUNDTRIP_REPO not set." >&2
  echo "  Export it (e.g. HUNKYARD_ROUNDTRIP_REPO=hunkyard-md/hunkyard-roundtrip-sandbox)." >&2
  echo "  See test/roundtrip/README.md for one-time setup." >&2
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "✗ gh not installed" >&2; exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "✗ gh not authenticated" >&2; exit 1
fi

echo "→ Building hunkyard..."
make -C "$HUNKYARD_DIR" build -j

export HUNKYARD_ROUNDTRIP_REPO
export HUNKYARD_BINARY="$HUNKYARD_DIR/hunkyard"

cd "$HUNKYARD_DIR"
exec go test -tags e2e_github -count=1 -timeout 20m -run TestRoundtrip "$@" ./...
