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

The canonical store is `${LEARN_PREFERENCES_HOME:-$HOME/.agents}`, shared by both hosts. Clear durable instructions can become active preferences; uncertain reusable feedback stays pending. Task-specific edits are ignored. Generated personas preserve repository/global/source precedence and token budgets.

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

`install-hook` registers one command per host event:

- `SessionStart` loads your core preferences once, with a health line when maintenance is due or failed, and says when a Hunkyard review this session launched has published feedback waiting.
- `UserPromptSubmit` adds rules relevant to what you typed and keeps them for the rest of the session. The first prompt after you interrupt the agent (Codex `Interrupt`, or Claude's "interrupted by user" and rejected tool calls) is recorded as a correction.
- `PreToolUse` checks `git commit` and `git push` against your guards, and when the agent edits a file it adds the rules for that file's area (for example Ruby, migrations, jobs, frontend) once per session.
- `Stop` delivers any review batch published while the agent worked, then runs mechanical checks on lines added this turn, including new untracked files. On Claude, a background `asyncRewake` handler also waits up to an hour for your next publication and wakes Claude when you publish. Inspect and trust the commands through the host's normal `/hooks` UI. Installers replace earlier Hunkyard hooks, preserve unrelated settings, back up changed configuration, and roll back failed writes. Infrastructure failures in a hook are advisory and never block the host. On Claude, install also adds Hunkyard spinner verbs and tips to your own and offers `hunkyard preferences statusline` as the status line when none is set.

Guards turn a preference into a block at action time. The agent is told which rule blocked it and why; only you can let one action through:

```sh
hunkyard preferences guard add --kind staged_paths --rule RULE_ID --pattern 'docs/superpowers/**' --pattern '*-plan.md' --json
hunkyard preferences guard add --kind no_coauthor_trailer --rule RULE_ID --json
hunkyard preferences guard add --kind commit_message_format --rule RULE_ID --pattern '^Refs #\d+: ' --json
hunkyard preferences guard add --kind no_unrequested_commit --rule RULE_ID --json
hunkyard preferences guard add --kind diff_check --rule RULE_ID --pattern ruby_single_line_def --json
hunkyard preferences guard add --kind no_coauthor_trailer --rule RULE_ID --match 'git commit -m "x\n\nCo-authored-by: a <a@b>"' --not-match 'git commit -m "x"' --json
hunkyard preferences guard list --json
hunkyard preferences guard test --json                 # re-run every guard's examples
hunkyard preferences check --cwd . -- git commit -m x  # would this be blocked? no side effects
hunkyard preferences allow RULE_ID --once    # run it yourself, e.g. with ! in the CLI
hunkyard preferences doctor --json          # hooks, trust, store, maintenance and cron
```

Maintenance uses a **local proposal file** prepared by the launching host:

```sh
hunkyard preferences maintain --dry-run --json
hunkyard preferences maintain --dry-run --proposal /path/to/proposal.json --json
hunkyard preferences maintain --due --proposal /path/to/proposal.json --json
```

Without a proposal, maintenance returns a digest-bound batch for the host to review. Automatic maintenance hands that batch to a model CLI you choose in `~/.hunkyard.config.json` (global config only):

```json
{"preferences_maintenance": {"host": "claude", "model": "claude-opus-5-5", "effort": "high"}}
```

`host` is `claude` or `codex`; `model` and `effort` default to `claude-opus-5-5`/`high` for Claude and `gpt-6.1-sol`/`high` for Codex. When it is set, the hourly cron job and session start run `hunkyard preferences maintain --auto` whenever observations are pending and today's maintenance has not succeeded. A rejected proposal is retried once with the validation error; every outcome is recorded and shown at session start, in `doctor`, and in the status line. Without the setting, the cron job only saves local requests and an agent session prepares the proposal. The Go engine validates evidence, scope, citations, action counters, and rule relationships before atomic publication. See [maintenance, migration, and recovery](integrations/preferences/references/local-maintenance.md).

## Local-only behavior

Hunkyard performs no telemetry, analytics, update checks, remote sharing, forge requests, or automatic downloads. It makes no provider requests itself; the one exception you opt into is automatic preference maintenance, which runs your configured `claude` or `codex` CLI. Browser libraries and fonts ship inside the binary. The supported runtime uses Git; automatic Sapling/Jujutsu selection and their command overrides are disabled. The process HTTP transport rejects external destinations before DNS/proxy use, and browser CSP restricts resources to local assets. Git runs without remote lazy fetching, external diff/textconv/filter helpers, or filesystem monitor hooks. Dirty comparisons involving Git clean/process filters (including LFS) are rejected with an explanation; use committed `--range` comparisons for those paths. Binary changes stay in the file list as metadata, with raw bytes preserved in private review snapshots; SVG is reviewed as source. Executable HTML previews and remote review commands are disabled.

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
