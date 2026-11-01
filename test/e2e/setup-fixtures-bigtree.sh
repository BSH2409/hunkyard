#!/usr/bin/env bash
# Bigtree fixture: a large working tree reviewed through selected code paths.
#   - 5,000 tracked files under src/ (main has three commits)
#   - a feature branch with three commits touching src/
#   - 300 untracked files under newpkg/
#   - 30,000 ignored files under build/
#   - 200 remote-tracking refs
# Serves git mode restricted to `src newpkg` for *.bigtree.spec.ts.
#
# Files come from one awk per class: a process per file is ~20-50x slower
# under Git Bash on Windows.
set -euo pipefail

PORT="${1:-3138}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HUNKYARD_SRC="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"
DIR=$(e2e_native_tempdir)
BIN_DIR=$(e2e_native_tempdir)
e2e_cleanup_install
e2e_cleanup_add "$DIR" "$BIN_DIR"

cd "$DIR"
git init -q -b main
git config user.email "test@test.com"
git config user.name "Test"
git config core.autocrlf false
git config core.eol lf

# 50 packages x 100 files.
mkdir -p src
for ((p = 0; p < 50; p++)); do mkdir -p "src/pkg$p"; done
awk 'BEGIN {
  for (p = 0; p < 50; p++) for (f = 0; f < 100; f++) {
    path = "src/pkg" p "/file" f ".go"
    printf "package pkg%d\n\nfunc F%d() int {\n\treturn %d\n}\n", p, f, f > path
    close(path)
  }
}'
printf 'build/\n' > .gitignore
git add -A
git commit -q -m "initial tree"
printf '// main 2\n' >> src/pkg0/file0.go
git commit -qam "main: second"
printf '// main 3\n' >> src/pkg0/file1.go
git commit -qam "main: third"
BASE2_SHA=$(git rev-parse main~2)

git checkout -q -b feature
for ((c = 1; c <= 3; c++)); do
  for ((f = 0; f < 10; f++)); do
    printf '\nfunc Feature%d_%d() int {\n\treturn %d\n}\n' "$c" "$f" "$c" >> "src/pkg$c/file$f.go"
  done
  git commit -qam "feature: commit $c"
done

# Untracked and ignored files.
mkdir -p newpkg
awk 'BEGIN {
  for (i = 0; i < 300; i++) {
    path = "newpkg/new" i ".go"
    printf "package newpkg\n\nfunc New%d() int {\n\treturn %d\n}\n", i, i > path
    close(path)
  }
}'
for ((d = 0; d < 30; d++)); do mkdir -p "build/out$d"; done
awk 'BEGIN {
  for (i = 0; i < 30000; i++) {
    path = "build/out" (i % 30) "/obj" i ".o"
    print "obj" > path
    close(path)
  }
}'

HEAD_SHA=$(git rev-parse HEAD)
awk -v sha="$HEAD_SHA" 'BEGIN { for (i = 0; i < 200; i++) print "create refs/remotes/origin/branch" i " " sha }' \
  | git update-ref --stdin

if [ -z "${HUNKYARD_BIN:-}" ]; then
  HUNKYARD_BIN="$BIN_DIR/$(e2e_bin_name)"
  (cd "$HUNKYARD_SRC" && go build -o "$HUNKYARD_BIN" ./cmd/hunkyard)
fi

FAKE_HOME=$(e2e_native_tempdir)
e2e_cleanup_add "$FAKE_HOME"
e2e_export_fake_home "$FAKE_HOME"

STATE_FILE="$(e2e_state_file "$PORT")"
{
  echo "HUNKYARD_BIN=$HUNKYARD_BIN"
  echo "HUNKYARD_FIXTURE_DIR=$DIR"
  echo "FAKE_HOME=$FAKE_HOME"
  echo "BASE2_SHA=$BASE2_SHA"
} > "$STATE_FILE"

echo '{"agent_cmd": "echo"}' > "$FAKE_HOME/.hunkyard.config.json"

e2e_serve "$HUNKYARD_BIN" _serve --no-open --port "$PORT" src newpkg
