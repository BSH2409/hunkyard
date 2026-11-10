# Hunkyard guide

Start with the [README](../README.md) for a visual walkthrough and quick start. This guide covers the review workflow, CLI integrations, preferences, and local storage in more detail.

## Install a release

On Linux or macOS, install the latest release without cloning the repository:

```sh
curl -fsSL https://raw.githubusercontent.com/BSH2409/hunkyard/main/install.sh | bash
```

The installer supports x86_64 and ARM64, including WSL. It checks for Git, curl, standard shell utilities, and a SHA-256 tool (`sha256sum` or `shasum`). It downloads the release binary and its checksums from GitHub over HTTPS, verifies the checksum, and checks that the binary runs before replacing an existing installation. A failed download or verification leaves the previous binary in place.

The default destination is `~/.local/bin`. When that directory is missing from PATH, the installer adds it to `~/.zshrc` for Zsh, `~/.bashrc` for Bash on Linux, or the first existing Bash login file (`~/.bash_profile`, `~/.bash_login`, or `~/.profile`) on macOS. If no Bash login file exists, it creates `~/.bash_profile`. Open a new terminal or run the printed command to update your current shell. Other shells receive instructions to configure PATH themselves. No sudo is used, and agent integrations are installed separately.

To pin a release or use another absolute install directory:

```sh
curl -fsSL https://raw.githubusercontent.com/BSH2409/hunkyard/main/install.sh | HUNKYARD_VERSION=v1.0.5 bash
curl -fsSL https://raw.githubusercontent.com/BSH2409/hunkyard/main/install.sh | HUNKYARD_INSTALL_DIR="$HOME/bin" bash
```

Run the installer again to update to the latest release. To uninstall, remove the installed `hunkyard` binary; review state and agent integrations remain in place.

Native Windows users can download the matching `.exe` from [GitHub Releases](https://github.com/BSH2409/hunkyard/releases/latest).

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

Hunkyard's browser API has no network authentication, so keep the published host port on loopback. Outside Docker, use `hunkyard --host 127.0.0.1` for local-only listening. See the [Docker build and run instructions](../integrations/docker/README.md).

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

### Review tools

- **Labels.** A line or file comment can carry a conventional-comment label (nit, issue, suggestion, praise, question) and a **blocking** flag. While a blocking comment is unresolved, Approve is disabled in the submit dialog.
- **Since last review.** In Git mode, the **All changes | Since last review** toggle shows only what changed after your last published review. Before the first publication it shows all changes and says so.
- **Changed since you viewed.** A file you marked Viewed comes back un-viewed with a badge when its content changes.
- **Claimed fixed, code unchanged.** When the agent replies to a comment but leaves the commented lines identical, the next round reopens the comment and badges it, and the published batch sets `claim_unchanged`.
- **Your own edits.** Lines you edit on disk during a review are published as `edit` comments (your version against the agent's), so the agent keeps them and can learn from them.
- **Tool results in the diff.** `hunkyard annotate` imports output from tools you already run: SARIF (golangci-lint, ESLint, Semgrep and others), `go test -json`, Go cover profiles and lcov. Findings become comments by the tool, and changed lines no test covered get a red edge, with a "2 of 9 changed lines uncovered" note in the file header. Running it again for the same tool replaces that tool's earlier findings and keeps any thread you replied to. Hunkyard only reads the file you pass; it never runs the tool. The **Tools** filter in the comments panel lists these comments.

  ```sh
  go test -coverprofile=cover.out ./... && hunkyard annotate --format coverprofile cover.out
  golangci-lint run --output.sarif.path=lint.sarif && hunkyard annotate --format sarif lint.sarif
  go test -json ./... | hunkyard annotate --format go-test-json -
  ```

- **Rule checks.** A learned preference bound to a `diff_check` guard is checked against the lines the review adds when it opens and at every new round. A match is posted as a **rule check** comment. **Confirm** keeps it open for the agent and counts as a violation of the rule; **Dismiss** resolves it and counts against the check in the Preferences tab. Checks: `ruby_single_line_def`, `ruby_semicolon_statement`, `js_console_log`, `todo_without_owner`, `skipped_test_added`.

Tool and rule comments carry `source` and `source_ref` in the published batch and are never preference evidence.

- **Impact.** In Git mode each code file's header has an impact pill such as "2 callers · 1 test". It counts other tracked files that mention the functions, types and constants the diff adds or removes (Go, JavaScript and TypeScript, Ruby, Python), split into callers and tests. Click it to list them; a click on a reference jumps there when the file is part of the review. These are text matches, not a call graph.
- **Overview and risk order.** The overview button in the header shows what is compared, the commits and the totals. **By risk** orders the file list and tree by a score you can read in the overview: sensitive paths (auth, secrets, migrations, payments and your `risk_patterns`), size of the change, no test changed next to it, uncovered lines and open rule findings.
- **Decisions from the agent.** When the agent needs you to choose, it runs `hunkyard ask --session ID --option A --option B 'Question?'`. Its questions open in a panel in the bottom-right corner, one at a time: pick an option (or press its number), or write your own answer, then move to the next. A last page lists every question with your answer, with **Change** links, and **Submit answers** sends them together. They reach the agent with the next published review as `decisions`. A `--blocking` question keeps Approve disabled until you answer it.

### Idle pause and updates

- **Idle pause.** A review with no activity for 30 minutes pauses: the daemon exits and frees its port, and the open tab shows **Review paused**. Clicking, typing or scrolling in the page counts as activity, and so does an agent turn while the preference hooks are installed. Ask your agent to resume (it runs `hunkyard --session REVIEW_ID`); the review comes back on the same port and the tab reconnects by itself. Comments survive the pause. Set the global config `idle_pause_minutes` (or `HUNKYARD_IDLE_PAUSE_MINUTES`) to change the time; `0` turns pausing off.
- **Refresh.** The page does not redraw when files change underneath it. When the working tree, index, HEAD or refs move, a **This review has updates** banner offers **Refresh**. Changing the base or picking a commit always loads the latest state. Agent review rounds still update automatically.

- **Inbox.** The inbox button in the header opens `/inbox`, a list of every review on this machine: which ones wait for you, where the agent is working, and which paused. A paused review has a **Resume** button. `hunkyard inbox` prints the same list.
- **Insights.** `hunkyard stats --days 30` (and the **Insights** view in the Preferences tab) shows how your reviews went: rounds per review, comments per round, time to approve, comments by label and resolution, repositories, and the learned rules most often behind your comments. Everything is computed from local files; `disable_stats` turns recording off.

### Large repositories

Opening a review and switching the base or a commit share one cached reading of the working tree, and diffs for many files come from one `git diff` call. If `hunkyard check` reports that `git status` is slow, enable Git's untracked cache:

```sh
git config core.untrackedCache true
```

See the [complete local protocol](../integrations/shared/review-workflow.md) for trusted skill catalogs, human comment revisions, preference classification, replies, acknowledgments, and repeated review rounds. It is CLI/JSON and does not depend on a host SDK.

## Go-backed coding preferences

The review bridge classifies every human comment revision through the Go preference engine, including summaries and human replies. It excludes agent messages and keeps classification checkpoints separate from delivery acknowledgments. Replay does not add duplicate evidence.

Learned preferences and the Preference Yard run on macOS and Linux (including WSL). On native Windows, reviews work but `hunkyard preferences` reports that learning is unsupported, installed hooks stay silent, and review feedback is acknowledged without learning.

The canonical store is `${HUNKYARD_PREFERENCES_HOME:-$HOME/.agents}`, shared by both hosts. Clear durable instructions can become active preferences; uncertain reusable feedback stays pending. Task-specific edits are ignored. Generated personas preserve repository/global/source precedence and token budgets.

Install the `hunkyard-preferences` skill separately (it replaces an older `learn-preferences` skill that Hunkyard installed):

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

### Team packs

Share conventions with your team as a **team pack**: a folder of markdown with one rule per bullet, grouped by surface, and a `pack.json` that pins its digest. Export yours, hand the folder to a teammate (or commit it), and import theirs:

```sh
hunkyard preferences pack export --name backend --version 1.0.0 --out ./backend-pack --json
hunkyard preferences pack import ./backend-pack --dry-run --json
hunkyard preferences pack import ./backend-pack --json
hunkyard preferences pack list --json
hunkyard preferences pack remove shared-backend --json
```

A repository can commit its own pack under `.hunkyard/preferences-pack/`; every developer's session start imports it for that repository only, and removing the folder removes its rules. To decline a repository's pack, run `hunkyard preferences pack remove repo-…`: it stays out until the pack's content changes. `hunkyard preferences export --scope repository --to conventions.md` writes your repository rules as one markdown file to start from. Personal rules always win over pack rules, which win over source defaults. A pack only adds directives: it cannot change guards, hooks or configuration.

### Preferences tab

A review's Settings has a **Preferences** tab (the Preference Yard). It shows:

- **Rules** with evidence and violation counts. You can retire a rule, promote a repository rule to global, or add a rule.
- **Pipeline**: captured observations and the candidate, active and retired rules, with a button to run maintenance while observations or candidates are waiting.
- **Persona**: the generated files agents receive, rendered as markdown.
- **Loads**: which rules each recent session received, and why. The reasons are core at session start, a surface matched in your prompt, kept from earlier, or a file being edited.
- **Activity** and **Health**.

Changes are accepted only from the review page itself (same-origin).

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

`host` is `claude` or `codex`; `model` and `effort` default to `claude-opus-5-5`/`high` for Claude and `gpt-6.1-sol`/`high` for Codex. When it is set, the hourly cron job and session start run `hunkyard preferences maintain --auto` whenever observations are pending and today's maintenance has not succeeded. A rejected proposal is retried once with the validation error; every outcome is recorded and shown at session start, in `doctor`, and in the status line. Without the setting, the cron job only saves local requests and an agent session prepares the proposal. The Go engine validates evidence, scope, citations, action counters, and rule relationships before atomic publication. See [maintenance, migration, and recovery](../integrations/preferences/references/local-maintenance.md).

### Live mode and HTML preview

Review a running app or a local HTML page instead of its source, and pin comments to elements on screen:

```sh
hunkyard live http://localhost:3000     # an app on this machine (also: hunkyard http://localhost:3000)
hunkyard preview docs/page.html         # a local HTML file (also: hunkyard docs/page.html)
```

Hunkyard serves the page through a proxy on the review port + 1 and injects its pin scripts into the responses; nothing in your app or repository changes. Only loopback origins are accepted. In Docker, publish both ports, for example `-p 127.0.0.1:5000:5000 -p 127.0.0.1:5001:5001`.

Each pin takes a screenshot of the whole page inside the frame, with the element boxed in red (`#FF0000`, a 4 px line with a 2 px white halo, 6 px outside the element). The agent receives the screenshot URL, the box's exact position and the pin's status: `current`, `outdated` when the element's text, surroundings, size or HTML changed, or `unchecked` when its page has not been opened since the last round. Screenshots stay in the review folder, are served only to the review page and the agent (never to another site, including the reviewed app), and are deleted with their pin or when the review is approved. Commenting again on exactly the same element adds a reply to its open pin.

## Local-only behavior

Hunkyard performs no telemetry, analytics, update checks, remote sharing, forge requests, or automatic downloads. It makes no provider requests itself; the one exception you opt into is automatic preference maintenance, which runs your configured `claude` or `codex` CLI. Browser libraries and fonts ship inside the binary. Git is the only supported version control system. The process HTTP transport rejects external destinations before DNS/proxy use, and the review page's CSP restricts it to local assets. A page under review (live mode or HTML preview) loads its own fonts, scripts and images as it does when opened directly; set `HUNKYARD_BLOCK_EXTERNAL=1` to keep it to local resources. Git runs without remote lazy fetching, external diff/textconv/filter helpers, or filesystem monitor hooks. Dirty comparisons involving Git clean/process filters (including LFS) are rejected with an explanation; use committed `--range` comparisons for those paths. Binary changes stay in the file list as metadata, with raw bytes preserved in private review snapshots. Changed PNG, JPEG, GIF and WebP files show the before and after images side by side, with **Slider** and **Onion skin** comparisons; the images are served only to the review page (10 MB per image). SVG is reviewed as source. Live mode and HTML preview run pages only from this machine (loopback origins and local files); remote review commands are disabled.

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

MIT licensed; see [LICENSE](../LICENSE).
