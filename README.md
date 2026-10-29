# Hunkyard

Local browser diff review for **Codex CLI and Claude CLI**.

Review working changes or a commit range before pushing. Add inline comments, select skills offered by the launching agent, and submit an explicit **Comment**, **Approve**, or **Request changes** decision. The launching CLI session receives the publication, applies the selected skills, records reusable coding preferences, and replies in the browser.

Hunkyard is one Go binary with embedded browser assets. It listens on **0.0.0.0:5000** by default and works across repositories. Open **http://localhost:5000** in your browser.

## Build and run

From this checkout, with Go 1.26 or newer:

```sh
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/hunkyard" ./cmd/hunkyard
```

Ensure `~/.local/bin` is on `PATH`, then run it from any repository:

```sh
hunkyard                              # working changes
hunkyard --range HEAD~3..HEAD          # exact comparison
hunkyard --range main...HEAD           # PR-style comparison from the merge base
hunkyard src/main.go src/auth.go       # selected code changes
hunkyard README.md                    # document review
```

The browser opens automatically. `--no-open` prints the URL. `--port 0` selects a free port; a busy default port is reported without stopping another process. Commit ranges resolve to full local object IDs. No push, fetch, remote URL, or GitHub account is required.

Changed code selected by file or directory opens the Git diff view by default, with added/modified status in the sidebar. Explicit Markdown reviews, unchanged files, and files outside Git open in document/source view.

### Docker with a host browser

Publish with `-p 127.0.0.1:5000:5000`, run `$hunkyard` in Codex, and open `http://localhost:5000` on the host. The default listener already accepts Docker forwarding; no network flags or environment variables are needed.

Hunkyard's browser API has no network authentication, so keep the published host port on loopback. Outside Docker, use `hunkyard --host 127.0.0.1` for local-only listening. See the [Docker build and run instructions](integrations/docker/README.md).

## Connect the launching CLI session

Install the host workflow once:

```sh
hunkyard install --global codex
hunkyard install --global claude
```

Use `$hunkyard` in Codex or `/hunkyard` in Claude. Installing a skill may require a new host session before it is discoverable. Existing integration files are preserved unless `--force` is supplied.

The skill waits locally while you review. Codex uses one persistent await process that renews bounded HTTP waits silently. Publishing in the browser completes that waiter; the same agent session processes the returned JSON. This uses the existing host's tools and permissions. Hunkyard does not type into a terminal, spawn another model session, or invoke a provider API. Launching the standalone binary without the host skill opens a browser review, but does not make an idle agent consume feedback automatically.

Codex normally supplies `CODEX_THREAD_ID`. Claude's skill supplies its current session ID. Other hosts can provide a stable `HUNKYARD_OWNER_ID`, such as `my-plugin:SESSION`, and implement the same protocol:

```sh
hunkyard feedback await --session REVIEW_ID --follow --json
hunkyard feedback learn --session REVIEW_ID --batch BATCH_ID --decisions /absolute/path/decisions.json --json
hunkyard feedback ack --session REVIEW_ID --batch BATCH_ID --json
```

Keep the same owner on every command. `--follow` waits until publication, cancellation, or an error and suppresses `pending` responses. `--timeout` (default 45s) bounds each HTTP wait, not the overall follow process. Ctrl+C stops the waiter without stopping the review daemon or acknowledging feedback. Omit `--follow` for a one-shot wait that can return `pending`; this means the browser review is still open. A timeout is never approval. After processing a change request, `hunkyard --session REVIEW_ID` starts the next review round. Queued publications survive a stopped daemon and an interrupted waiter.

See the [complete local protocol](integrations/shared/review-workflow.md) for trusted skill catalogs, human comment revisions, preference classification, replies, acknowledgments, and repeated review rounds. It is CLI/JSON and does not depend on a host SDK.

## Go-backed coding preferences

The review bridge classifies every human comment revision through the Go preference engine, including summaries and human replies. It excludes agent messages and keeps classification checkpoints separate from delivery acknowledgments. Replay does not add duplicate evidence.

The canonical store remains `${CODEX_HOME:-$HOME/.codex}`, shared by both hosts. Clear durable instructions can become active preferences; uncertain reusable feedback stays pending. Task-specific edits are ignored. Generated personas preserve repository/global/source precedence and token budgets.

A separate installation replaces the `learn-preferences` workflow with the Go-backed skill:

```sh
hunkyard install --global preferences
hunkyard preferences select --query 'Implement a Go API with tests' --cwd "$PWD" --json
hunkyard preferences search --query 'naming' --json
hunkyard preferences audit --duplicates --json
```

Back up a customized existing skill before using `--force`. Skill installation does not change hooks, cron, or preference records. Use explicit installer commands if needed:

```sh
hunkyard preferences install-hook --host codex --json
hunkyard preferences install-hook --host claude --json
hunkyard preferences install-cron --json
```

Inspect and trust a hook through the host's normal `/hooks` UI. Installers preserve unrelated settings, back up changed configuration, and roll back failed writes. Hook failures are advisory and do not block the host.

Maintenance uses a **local proposal file** prepared by the launching host:

```sh
hunkyard preferences maintain --dry-run --json
hunkyard preferences maintain --dry-run --proposal /path/to/proposal.json --json
hunkyard preferences maintain --due --proposal /path/to/proposal.json --json
```

Without a proposal, maintenance returns a digest-bound batch for the host to review. An hourly cron job only saves local requests; it does not call a model. The Go engine validates evidence, scope, citations, action counters, and rule relationships before atomic publication. See [maintenance, migration, and recovery](integrations/preferences/references/local-maintenance.md).

## Local-only behavior

Hunkyard performs no telemetry, analytics, update checks, remote sharing, forge requests, provider requests, or automatic downloads. Browser libraries and fonts ship inside the binary. The supported runtime uses Git; automatic Sapling/Jujutsu selection and their command overrides are disabled. The process HTTP transport rejects external destinations before DNS/proxy use, and browser CSP restricts resources to local assets. Git runs without remote lazy fetching, external diff/textconv/filter helpers, or filesystem monitor hooks. Dirty comparisons involving Git clean/process filters (including LFS) are rejected with an explanation; use committed `--range` comparisons for those paths. Binary changes stay in the file list as metadata, with raw bytes preserved in private review snapshots; SVG is reviewed as source. Executable HTML previews and remote review commands are disabled.

The launching Codex/Claude session still communicates with its model provider as usual. Its tools and permissions remain under the host's control. Hunkyard itself only exchanges local HTTP and private files. This is an application-level boundary, not an operating-system network sandbox.

Review state lives under `~/.hunkyard`; credentials use private owner files and HTTP headers, never browser storage, URLs, or command arguments. Preference generations and compatibility mirrors use the existing canonical store. Do not hand-edit generated files.

## Development

```sh
go test ./...
go test -race ./internal/preferences ./internal/feedback ./internal/session ./cmd/hunkyard
npm ci --ignore-scripts --no-audit --no-fund
npm run test:frontend
npm run verify-assets
```

Node/npm are development tools; running the built binary needs neither Node nor Ruby. Vendored assets are already checked in. Browser acceptance tests live under `test/e2e` and use temporary repositories and homes.

The compatibility inventory under `internal/preferences/testdata/contracts` maps 274 Ruby reference cases to Go regressions and documents 10 replacements. Provider processes and Ruby wrappers are replaced by local proposal files and the Go CLI; catalogs are private Go APIs; read-only failures leave the inbox and quarantine untouched. This accounts for all 284 cases without claiming identical provider behavior. GitHub diff colors are checked against a pinned public reference in both layouts and themes; whole-page pixel parity is not established. Native Windows hook/transcript behavior and the Nix package need separate acceptance.

MIT licensed; see [LICENSE](LICENSE).
