# Hunkyard — Development Guide

Single-binary Go CLI that opens a browser-based UI for reviewing code changes and markdown files with GitHub PR-style inline commenting. Multi-file review with git diff rendering and structured review file output for AI coding agents.

## Project map

```
hunkyard/
├── cmd/hunkyard/            # package main — thin CLI (main.go, cli_*.go, wire.go)
├── internal/            # Core logic packages (daemon, server, session, github, vcs, …)
├── web/                 # Embedded frontend assets (Go package webassets; embed.go)
│   ├── index.html       # HTML shell — code-review OR live-mode script fork
│   ├── app.js           # Code-review mode JS
│   ├── live-mode*.js    # Live-mode modules
│   ├── hunkyard-agent.js / agent-*.js / agent-marker.css  # Injected iframe scripts
│   ├── hunkyard-*.js        # Shared renderer, SSE, draft, comment UI modules
│   ├── style.css / style-live.css / theme.css
│   ├── __tests__/       # Node.js unit tests (node --test)
│   ├── pierre/          # Vendored @pierre/diffs + Shiki (split ESM, gzip; scripts/build-pierre.mjs)
│   ├── katex/           # Vendored KaTeX (JS, CSS, woff2 fonts) for Markdown math, loaded on first use
│   └── *.min.js         # Vendored markdown-it, DOMPurify, mermaid, diff-match-patch, gemoji (emoji shortcodes)
├── integrations/        # Drop-in config files for AI coding tools (claude-code, codex)
├── test/                # Test harnesses, E2E, roundtrip docs, shared fixtures
├── Makefile
├── package.json
└── copy-deps.js         # Copies npm deps into web/ for embedding
```

## Key architecture decisions

1. **All frontend assets embedded** via Go's `embed.FS` — produces a true single binary
2. **No frontend build step** — vanilla JS, no npm/webpack/framework. npm is only for fetching vendor libs.
3. **Two modes**: "git" mode (auto-detect changes, optionally restricted to selected code paths) and "files" mode (explicit Markdown documents, unchanged files, or files outside Git)
4. **markdown-it for parsing** — chosen because it provides `token.map` (source line mappings per block)
5. **Block-level splitting** — lists, code blocks, tables, blockquotes split into per-item/per-line/per-row blocks so each source line is independently commentable
6. **Diff rendering is `@pierre/diffs`** — the review list (git and files mode) is one Pierre `CodeView`: virtualization, sticky headers, Shiki highlighting in a worker pool, word diffs, split/unified, context expansion. Hunkyard supplies the file header, comments/forms as annotations, keyboard focus and jumps. Rendered markdown (Document view) is Hunkyard's line-block document inside a Pierre item. Story chapters use Pierre `FileDiff`. See `docs/frontend-js.md`.
7. **Comments reference source line numbers** — stored in `~/.hunkyard/reviews/<key>.json` with per-file sections
8. **Real-time output** — review file written on every comment change (200ms debounce)
9. **File watching** — git mode polls `git status --porcelain`; files mode polls mtimes; reloads via SSE
10. **Docker-ready listener** — server binds to `0.0.0.0:5000` by default; browser URLs use localhost and CLI requests use loopback. Use `--host 127.0.0.1`, `HUNKYARD_HOST`, or global `host` for local-only listening. Network binding is enabled by default and can be disabled with `HUNKYARD_ALLOW_UNAUTHENTICATED_NETWORK=0`. The browser API has no network auth; publish Docker ports on host loopback.
11. **Two-level config** — `~/.hunkyard.config.json` (global) merged with `.hunkyard.config.json` (project), CLI flags override both. `agent_cmd` and `plan_approve_mode` are global-only (prevents malicious repos from hijacking agent commands or weakening Claude Code permissions)
12. **Headless CLI comment** — `hunkyard comment` writes directly to the review file without starting the server; SSE notifies any running server
13. **Comment threading** — comments support nested replies and a `resolved` boolean. Review file schema nests replies inside each comment's `replies` array.
14. **Centralized review storage** — `~/.hunkyard/reviews/<key>.json` keyed by cwd + branch (git mode) or cwd + args (file mode)
15. **VCS abstraction** — `vcs.go` defines a backend interface; `git_vcs.go`, `sapling.go`, and `jj.go` are the implementations. Auto-detected, overridable via `--vcs` flag or `vcs` config key. Subcommands not yet threaded through (see TODO at `main.go:1826`).
16. **Pluggable renderers** — `web/hunkyard-renderers.js` registry; two registration kinds: `kind: 'fence'` (by info string, `langs`) and `kind: 'file'` (by glob, `paths`, `<label> / Source` toggle, `viewMode: 'rendered'`). Built-in mermaid (`hunkyard-renderer-mermaid.js`) covers ```` ```mermaid ````, `*.mermaid`, `*.mmd`, and injects `mermaid.min.js` only on the first diagram (not in `index.html`). New renderers ship in `web/` (no external script loading). See `docs/renderers.md`.
17. **Focus mode** — sub-views over the file list: file focus, range focus (`--range A..B`), stacked focus (range layer in a stacked PR). Lives in `focus_*.go` and `/api/focus`.

<important if="you are writing a plan, design doc, or implementation proposal, or about to commit">
Do not commit plan files to the repo — keep them as untracked local files (or in `/tmp`). This includes `*-plan.md`, `*-proposal.md`, and other AI-generated design docs. Repo history should contain implementation, not planning artifacts. Exception: test fixtures under `test/` that a test explicitly reads.
</important>

<important if="you need to build, test, lint, or run hunkyard">

```bash
go build -o hunkyard ./cmd/hunkyard                           # Build
go test ./...                                         # Run all tests
gofmt -l .                                            # Check formatting (should be clean)
golangci-lint run ./...                               # Lint (should be clean)
make build-all                                        # Cross-compile to dist/
./hunkyard                                                # Git mode (auto-detect changed files)
./hunkyard test-plan.md                                   # Review specific file(s)
./hunkyard --no-open --port 3000 test-plan.md             # Headless on fixed port
```
</important>

<important if="you need to know what hunkyard subcommands do or are adding/modifying a CLI subcommand">

Subcommands are dispatched via `commandDispatch` in `main.go`. Anything not in the table falls through to `runReview`.

```
hunkyard                          # Review git changes (starts daemon, blocks for feedback)
hunkyard <file|dir> [...]         # Review specific files or directories (falls through to runReview)
hunkyard review [...]             # Explicit review invocation (same as default)
hunkyard live <url>               # Review a running web app in live mode (also: hunkyard <url>)
hunkyard preview <file.html>      # Review a local HTML file in preview mode (also: hunkyard <file.html>)
hunkyard stop [--all]             # Stop daemon for current directory; --all stops every daemon
hunkyard status [--json]          # Show review file path, daemon status, comment stats
hunkyard resume [--list | <id>]   # Pick a stored review to reopen (interactive list of ~/.hunkyard/reviews)
hunkyard cleanup [--days N] [--force]  # Delete stale review files from ~/.hunkyard/reviews/
hunkyard pull [pr-number]         # Fetch GitHub PR comments into the review file
hunkyard push [--dry-run] [--event <type>] [-m <msg>] [pr]  # Post review comments as a GitHub PR review
hunkyard pr <num|url>             # Thin shim — forwards to `hunkyard review --pr <n>`
hunkyard comment <path>:<line[-end]> <body>         # Add a comment (no server needed)
hunkyard comment --selector <css> [--route <path>] <body>  # Pin a DOM element (live or HTML preview)
hunkyard comment --reply-to <id> [--resolve] <body> # Reply to a comment
hunkyard comment --json [--file <path>] [--author <name>]  # Bulk add comments from JSON (stdin or --file; - = stdin)
hunkyard config [--generate]      # Print resolved config (or starter template)
hunkyard install <agent>          # Install integration config for an AI tool
hunkyard plan [...]               # Plan-file workflow
hunkyard plan-hook [--mode claude|codex]  # Internal hook used by agent plan flows
hunkyard check                    # Self-check (env, git, gh availability)
hunkyard yard [--port N] [--host H] [--no-open]  # Preference Yard: rules, loads, activity, health (no review session)
hunkyard _serve                   # Internal: foreground server (used by daemon spawn)
hunkyard --version | -v           # Version
hunkyard help | --help | -h       # Show help
```
</important>

<important if="you are working with config files (~/.hunkyard.config.json or .hunkyard.config.json) or adding a config key">

Two-level JSON config files, merged (project overrides global):

- **Global**: `~/.hunkyard.config.json` — user-wide defaults
- **Project**: `.hunkyard.config.json` in repo root — per-project overrides

Config keys: `port`, `host`, `no_open`, `quiet`, `output`, `author`, `base_branch`, `ignore_patterns`, `auto_viewed_patterns`, `default_markdown_view`, `agent_cmd`, `plan_approve_mode`, `cleanup_on_approve`, `notify_on_round_ready`, `disable_stats`, `no_integration_check`, `vcs`, `live_cookie`, `live_cookie_file`, `live_cdp_url`, `close_on_approve_after_ms`, `stale_review_days`.

- `base_branch` overrides auto-detected default branch (used as diff base in git mode, and by `hunkyard pull`/`hunkyard push`/`hunkyard comment`)
- `author` falls back to the configured VCS user name if not set
- `agent_cmd`, `plan_approve_mode`, `close_on_approve_after_ms`, and `stale_review_days` are **global config only**; project-level config cannot override (security — prevents malicious repos from hijacking the agent command, weakening Claude Code permissions, or forcing a reviewer's tab to auto-close, or shortening review retention for every project)
- `close_on_approve_after_ms` (default: unset/disabled) — auto-close the review tab N ms after Approve with no unresolved comments; negative values are treated as unset. Not included in `hunkyard config --generate` scaffolding.
- `cleanup_on_approve` (default: `true`) — auto-delete review file when reviewer approves with no unresolved comments
- `stale_review_days` (default: `14`) — age in days after which the background sweep started by `hunkyard review` / `hunkyard plan` deletes an untouched review; unset or non-positive values use the default. Not included in `hunkyard config --generate` scaffolding.
- `notify_on_round_ready` (default: `false`) — opt in to a desktop notification when a review round becomes ready for the human. On macOS, install `terminal-notifier` so the notification's click action opens the review URL; without it, clicking falls back to AppleScript `display notification`, which macOS attributes to Script Editor instead of the browser
- `no_update_check` is still accepted for older configs but has no effect: Hunkyard never checks for updates
- `disable_stats` (default: `false`) — disable session stats recording to `~/.hunkyard/stats.json`
- `ignore_patterns` are unioned (global + project both apply); types: `*.ext`, `dir/`, `exact.file`, `path/*.ext`
- `auto_viewed_patterns` are unioned (global + project both apply); matched client-side against file paths and applied once per launch to auto-mark matching files viewed (collapsed). No runtime default (empty). Plumbed through `/api/config` only — Go does no glob matching.
- `default_markdown_view` (`"diff"` | `"document"`, default unset = diff in git mode) — initial view for markdown files (Document/Diff toggle) in git mode. Project **overrides** global (scalar, not unioned). Invalid values are ignored with a stderr warning. No CLI flag. Plumbed through `/api/config` only.
- `vcs` selects backend: `"git"` (default), `"sl"` (sapling), or `"jj"` (Jujutsu)
- `live_cookie` / `live_cookie_file` forward session cookies to the upstream app in live mode (global or project; prefer gitignored `live_cookie_file` e.g. `.hunkyard/live-cookies.txt`). CLI: `hunkyard live --cookie`, `--cookie-file`
- `live_cdp_url` reuses cookies from a local Chrome DevTools endpoint (global or project). CLI: `hunkyard live --cdp-url`. Explicit `--cookie` values override CDP cookies with the same name.
- CLI flags override config file values
</important>

<important if="you are working with hunkyard pull, hunkyard push, or GitHub PR sync">

Requires `gh` CLI installed and authenticated.

- `hunkyard pull` fetches PR review comments (RIGHT-side only) and merges them into the review file, deduplicating by author+lines+body
- `hunkyard push` reads the review file and posts unresolved comments as a GitHub PR review
- `hunkyard push --dry-run` shows what would be posted without creating the review
- `hunkyard push --event approve` submits an approval; `--event request-changes` requests changes (default: `comment`)
- `hunkyard push -m 'message'` adds a review-level body message
- PR number auto-detected from current branch, or pass explicitly: `hunkyard pull 42`
- Any code path that imports comments from an external source (GitHub PR) into the local review file MUST dedup against local state first (`mergeGHComments` dedups by author+lines+body). Merging without it causes duplicate comments on repeated pull.
</important>

<important if="you are writing, running, or modifying Playwright E2E tests in test/e2e/">

The `test/e2e/` directory contains Playwright tests against a real compiled `hunkyard` binary — no mocking.

### Running

```bash
make e2e                                              # Full suite
cd test/e2e && npx playwright test tests/comments.spec.ts  # One file
cd test/e2e && npx playwright test --headed                # Visible browser
E2E_DEBUG=1 make e2e                                  # Enable video + trace capture on failure
make e2e-report                                       # View HTML report with screenshots
```

### Projects

Nine Playwright projects. Test naming convention determines which project runs which file:

| Project | Port | Fixture | Test glob |
| --- | --- | --- | --- |
| `git-mode` | 3123 | `setup-fixtures.sh` (git repo + feature branch) | `*.spec.ts` (excludes other suffixes) |
| `mobile` | 3123 (3131 under `run.sh`) | `setup-fixtures.sh` (reuses a git-mode fixture) at 375x812, `hasTouch: true` | `*.mobile.spec.ts` |
| `file-mode` | 3124 | `setup-fixtures-filemode.sh` (plain files, no git) | `*.filemode.spec.ts` |
| `single-file-mode` | 3125 | `setup-fixtures-singlefile.sh` (one markdown file) | `*.singlefile.spec.ts` |
| `multi-file-mode` | 3127 | `setup-fixtures-multifile.sh` (code + markdown files) | `*.multifile.spec.ts` |
| `range-mode` | 3128 | `setup-fixtures-range-mode.sh` (`--range A..B` stacked git) | `*.rangemode.spec.ts` |
| `live-mode` | 3129 | `setup-fixtures-livemode.sh` (Go upstream + hunkyard live) | `*.livemode.spec.ts` |
| `perf` | 3134 | `setup-fixtures-perf.sh` (300 files + large markdown) | `*.perf.spec.ts` |
| `huge` | 3136 | `setup-fixtures-huge.sh` (~2,500 files; document taller than 2^22px) | `*.huge.spec.ts` |

The `mobile` project shares a git-mode fixture. `run.sh` runs git-mode as two shards on two fixtures (3123, 3131) and starts mobile on the 3131 fixture only after git shard 2/2 finishes, so the two never race on shared comment state (both `DELETE /api/comments` in `beforeEach`).

`run.sh` starts each project as soon as its own fixture answers `/api/session` (it fails fast if a fixture exits or isn't ready within `E2E_READY_TIMEOUT`, default 300s). Fixture scripts must use the `lib.sh` lifecycle helpers — `e2e_cleanup_install`, `e2e_cleanup_add <tmp dirs>`, `e2e_spawn` for helper servers, `e2e_serve "$HUNKYARD_BIN" _serve …` instead of `exec` — so a SIGTERM from `run.sh`/Playwright stops hunkyard and deletes the temp dirs. `e2e_export_fake_home` also sets `HUNKYARD_NO_INTEGRATION_CHECK=1` and `HUNKYARD_NO_UPDATE_CHECK=1`. Each `run.sh` project writes its screenshots and traces to `test-results/<name>/` (`git-1`, `mobile`, …), and `run.sh` merges their `.last-run.json` files so `make e2e-failed` reruns every failure. Generate fixture files with shell builtins or one `awk`, never a process per file (Git Bash on Windows makes each spawn ~20-50x slower). `perf` and `huge` fail on CI when a test only passes on retry (`failOnFlakyTests`).

CI runs E2E on PRs via `.github/workflows/test.yml`; a separate `coverage.yml` uploads unit coverage on push to `main`. Failed test artifacts are uploaded.

### Best practices

- **Never `waitForTimeout` / `setTimeout`** for state. Use auto-retrying assertions (`toPass()`, `toHaveClass()`, `toBeVisible()`). Sleep is OK only inside polling loops where you're already retrying.
- **Never `.count()` followed by `expect(count).toBe(N)`** — that's a snapshot. Use `await expect(locator).toHaveCount(N)` or wrap in `toPass()`.
- **Always import shared helpers from `./helpers`** (the file is `test/e2e/tests/helpers.ts`, plus `range-helpers.ts` for range-mode tests): `clearAllComments`, `loadPage`, `mdSection`, `goSection`, `jsSection`, `switchToDocumentView`, `dragBetween`, `clearFocus`, `addComment`, `getMdPath` (and `rangeFixture`, `ensureRangeFocus`, `ensureStackedFocus` from `range-helpers`). Don't redefine locally. Use `Page` types, not `any`.
- **Always call `clearAllComments(request)` in `beforeEach`** — server persists comments across tests. This calls `DELETE /api/comments` (bulk endpoint).
- **Markdown defaults**: git mode → diff view (call `switchToDocumentView()`); file mode → document view (no toggle).
- **Parallel execution**: projects run in parallel via shell. Within a project, tests run sequentially (`workers: 1`) — don't change this; they share server state.
- **Scroll before interact**: in file-mode (multiple files below the fold), call `scrollIntoViewIfNeeded()` before hover/click/drag.
- **CSS selectors**: check existing tests for class names (e.g. `.tree-comment-badge`, not `.tree-file-comments`).
</important>

<important if="you are modifying hunkyard pull, hunkyard push, GitHub PR comment sync, the review-file ↔ GitHub roundtrip, or anything in `github.go` / `pr_cache.go` / `pr_fetch_test.go` / `push_buckets.go` / `comment_cli.go` reply handling">

`roundtrip_integration_test.go` (build tag `e2e_github`) exercises the hunkyard ↔ GitHub PR roundtrip against a real sandbox PR. When modifying pull/push, GitHub-comment-bucket logic, reply posting, or `mergeGHComments*` dedup:

1. Run: `make e2e-roundtrip` (or `./scripts/e2e-roundtrip.sh -run <TestName> -v` for one scenario)
2. Add new `TestRoundtrip_<Name>` scenarios for new state transitions — see `test/roundtrip/README.md` for authoring notes
3. If a scenario is currently `t.Skip`'d against an issue and your change fixes the underlying bug, REMOVE the skip and run the scenario

Requires `gh` authenticated and `HUNKYARD_ROUNDTRIP_REPO=<owner>/hunkyard-roundtrip-sandbox` exported. Each scenario opens-then-closes a real PR (~10-25s each, ~100s suite). Tests are local-only (build tag keeps them out of CI / default `go test ./...`).
</important>

<important if="you are adding or modifying HTTP API endpoints in server.go">

All routes wrapped with `s.withReady` return 503 until session init completes — except `/api/health`.

Session-scoped:

- `GET  /api/health` — liveness probe (no readiness gate; used for daemon health checks); `{status, browser_clients, api_version}` — bump `APIVersion` only for breaking HTTP API changes; missing `api_version` = 0
- `POST /api/shutdown?pid=<daemon pid>` - graceful stop, same path as SIGTERM; CLI-only (403 on `Sec-Fetch-Site`/`Origin`), 409 when `pid` is another daemon's (checked before readiness, so also during init)
- `GET  /api/session` — session metadata
- `GET  /api/config` — `{version, installation_source, ...}`
- `GET  /api/review-cycle` — review-cycle metadata (round number, edits-since-last)
- `POST /api/finish` — write review file, return prompt for agent
- `GET  /api/events` — SSE stream (file-changed, edit-detected, server-shutdown)
- `GET  /api/wait-for-event` — long-poll until finish (used by `hunkyard` daemon mode)
- `POST /api/round-complete` — agent signals all edits done; triggers new round
- `…/api/focus` — set/clear focus (file or range scope)
- `…/api/picker` — file-picker UI backend
- `POST /api/agent/request` — send comment to configured `agent_cmd`
- `GET  /api/branches` — list local branches (for base-branch picker)
- `GET|POST /api/base-branch` — read/update active base branch
- `GET  /api/commits` — list commits between base ref and HEAD (git mode only)
- `GET  /api/files/list` — list session files (lighter than `/api/session`)
- `GET  /api/files/comments` — every file's visible comments keyed by path (`?round=N` in files mode); one request instead of one per file
- `GET|POST /api/comments` — list/add review-level comments
- `PUT|DELETE /api/review-comment/{id}` (and `/replies[/{rid}]`, `/resolve`) — review-comment CRUD

File-scoped (require `?path=X`):

- `GET  /api/file?path=X` — file content + metadata
- `GET  /api/file/diff?path=X` — diff hunks (git diff for code; inter-round diff for markdown)
- `GET|POST /api/file/comments?path=X` — list/add comments (10MB body limit on POST)
- `PUT|DELETE /api/comment/{id}?path=X` — update or delete (10MB body limit on PUT)
- `POST|PUT|DELETE /api/comment/{id}/replies[/{rid}]?path=X` — reply CRUD
- `PUT /api/comment/{id}/resolve?path=X` — set resolved state

Static: `GET /files/<path>` — serve files from repo root (path traversal protected). `GET /` — embedded frontend assets.
</important>

<important if="you are modifying server security, request handling, or path-validation logic">

- Server binds to `0.0.0.0:5000` by default for Docker port forwarding. Use `--host 127.0.0.1` or `HUNKYARD_HOST=127.0.0.1` to restrict listening, including when resuming a stopped review. `HUNKYARD_ALLOW_UNAUTHENTICATED_NETWORK=0` refuses non-loopback binding unless the CLI allow flag overrides it. Project config cannot change the listen host or network permission. Publish Docker ports with `-p 127.0.0.1:port:port` because the browser API is unauthenticated.
- State-changing requests (POST/PUT/PATCH/DELETE) with a `Sec-Fetch-Site` header must be `same-origin`. Missing header is allowed (CLI/curl/agent). `cross-site` is rejected — CSRF defense against malicious pages posting to loopback. Complements `checkHost` (DNS-rebinding); does not authenticate network clients.
- `/files/` validates paths, blocks `..` traversal, verifies resolved path stays within repo root
- Body size: 10MB for comments via `http.MaxBytesReader`
- HTTP server: `ReadTimeout: 15s`, `IdleTimeout: 60s` (no `WriteTimeout` — SSE needs open connections)
- Comment renderer uses `html: true` then DOMPurify with a pinned GitHub-compatible allowlist (`web/comment-html.js`) before `innerHTML` — never trust raw comment HTML
- Document renderer uses `html: true` intentionally (reviewing local files)
</important>

<important if="you are modifying web/ — app.js, style.css, theme.css, or index.html">

Frontend split: `index.html` (HTML shell), `app.js` (all logic), `style.css` (layout/components), `theme.css` (theme variables).

### Multi-file state model

Three top-level globals in `app.js`: `session` (mode, branch, base_ref, review_round, files), `files` (per-file render state with comments, lineBlocks), `activeForms` (multiple comment forms can be open simultaneously). See top of `app.js` for shapes.

### Source line mapping (markdown)

1. Parse with `markdown-it` to get tokens with `token.map` (source line ranges)
2. `buildLineBlocks()` dispatches to per-token-type handlers: `handleFenceToken`, `handleListToken`, `handleTableToken`, `handleBlockquoteToken`
3. Container tokens (lists, tables, blockquotes) are drilled into — each item/row/child becomes its own block
4. Code blocks (`fence` tokens) split into per-line blocks, highlighted by Shiki (`hunkyard-code-highlight.js`; grammars tokenized in Pierre's worker pool while the file loads)
5. Each block gets a gutter entry with its source line number(s)
6. Comments are keyed by `end_line` and displayed after their referenced block

### Diff rendering (code files)

`@pierre/diffs` renders every diff and code file (`web/hunkyard-pierre-view.js` wraps CodeView, `web/hunkyard-pierre-adapter.js` maps Hunkyard's model). Pierre's DOM lives in open shadow roots: lines are `[data-line]`, gutters `[data-column-number]`, the hover `+` is `[data-utility-button]`. Hunkyard content (header, comment cards, forms, rendered documents) is light-DOM annotations. Don't reach into the shadow DOM except through the existing helpers (selection, quote highlights, line tints).

### Known complexities

- `markdown-it` token.map quirks: last list item often claims a trailing blank line — code trims trailing blank lines from item ranges.
- Table separators (`|---|---|`): not in tokens, appear as gap lines. Detected via regex and hidden with CSS.
- Table rows remain separate source blocks for comments. Document view groups
  them into one native auto-layout `<table>`; rendered diff paths retain
  standalone row HTML as a compatibility fallback.
- CodeView virtualizes the file list: an off-screen file has no DOM. Navigate through `pierreView.scrollToFile/scrollToLine` (or the tree) rather than assuming a file's elements exist. Only a mounted rendered document keeps the classic `#file-section-<path>` wrapper.
- E2E: act on Pierre lines through the helpers (`hoverLine`, `openLineComment`, `revealFile`); they scroll the line into view and settle first.
</important>

<important if="you are changing any agent-*.js, hunkyard-agent.js, or agent-marker.css in web/">

These files are the scripts hunkyard injects into live/preview iframes — the canonical set + order is `agentScriptFiles` in `server.go`, plus `agent-marker.css` (served at `/agent-marker.css`).
</important>

<important if="you are adding CSS variables or modifying theme.css">

Header has a 3-button theme pill (System / Light / Dark):

- No `data-theme` attribute → system preference via `prefers-color-scheme`
- `data-theme="light"` / `data-theme="dark"` → explicit override
- CSS vars are set in `:root` (dark fallback), `@media (prefers-color-scheme: light) html:not([data-theme])`, `[data-theme="dark"]`, and `[data-theme="light"]` blocks. **Define every new variable in all four blocks.**
- Theme choice persisted via `hunkyard-settings` cookie (`theme` key, `"system"` | `"light"` | `"dark"`).
- Use CSS custom properties from `theme.css` for all colors. Never hardcode hex values.
</important>

<important if="you are modifying multi-round logic, round-complete, or finish handling">

When the agent runs `hunkyard` again (or calls `POST /api/round-complete`):

- **Markdown files**: snapshot content, carry forward unresolved comments, re-read from disk
- **Code files**: re-run git diff against base ref to get updated hunks
- **File list**: re-run `ChangedFiles()` to detect new/removed files
- Waiting modal shows live count of file edits while the agent works
- Diff toggle for markdown shows inter-round changes
</important>

<important if="you are modifying daemon spawning, session lookup, or ~/.hunkyard/sessions/">

`hunkyard` manages a background daemon for seamless multi-round reviews:

1. **First `hunkyard`**: starts background daemon (`hunkyard _serve`), opens browser, blocks for feedback
2. **Subsequent `hunkyard`**: connects to existing daemon (same cwd + args), signals round-complete, blocks
3. **`hunkyard plan.md`**: looks up daemon by hash(cwd + "plan.md") — reuses if alive, starts new if dead
4. **Ctrl+C**: kills the daemon the client started
5. **`hunkyard stop`**: stops daemon for current cwd; `hunkyard stop --all` stops every daemon. Both ask for a graceful stop via `POST /api/shutdown` first and fall back to signals
6. **Approve**: the approving client asks for `POST /api/shutdown`, waits for the daemon to exit, and only then applies `cleanup_on_approve`, under the session lock and only if no other daemon took the key over. A stopping daemon can still write the review, so never remove it earlier
7. **Lifetime**: daemon runs until approved or killed (Ctrl+C, `hunkyard stop`, or SIGINT/SIGTERM/SIGHUP). No idle timeout — walking away from a review session is fine.

### Deferred initialization & readiness

The daemon signals readiness (via OS pipe) as soon as the HTTP port is bound, but session init (git, file reads) continues in the background. Until `SetSession()` is called, most endpoints return **503 Service Unavailable**.

**Any client connecting to a daemon must poll `/api/session` until it stops returning 503 before calling other endpoints.** See `runReviewClient` and `runReviewClientRaw` for the canonical readiness loop. Skipping this poll causes races where endpoints return 503, and error-fallback paths may silently allow/approve when they shouldn't.

### Session registry

Daemon state in `~/.hunkyard/sessions/`, one file per session.
- Git mode (no args): `sha256(cwd + "\0" + branch)[:12]`
- File mode (args present): `sha256(cwd + "\0" + args...)[:12]` (branch excluded — file reviews aren't branch-dependent)

Session file: `{"pid", "port", "cwd", "args", "branch", "review_path", "started_at"}`. Review data lives at `~/.hunkyard/reviews/<key>.json` (same key).

`hunkyard _serve` runs the server in foreground (used by daemon spawning, not user-facing).

### Resuming a dead session

The session file is deleted as soon as its daemon dies, so a stopped session can only be found through its review folder. `review.json` therefore records `cwd` alongside `cli_args`: the session key is a hash of the cwd, so the directory cannot be recovered from the key. `hunkyard resume` lists the folders under `~/.hunkyard/reviews/` and hands the chosen key to `hunkyard --session <id>`, which rebuilds the `_serve` argv (`daemonArgsForReconnect`) and respawns the daemon in the recorded directory via `StartDaemonInDir`. Reviews written before the `cwd` field existed resume in the current directory, as they always did.
</important>

<important if="you are reviewing code or evaluating audit findings for this project">

Calibrate against the tool's actual scale before flagging issues. False-positive filters:

- **"Real problem at this scale?"** Localhost-only, single-user CLI. Patterns that matter for cloud services (context propagation, map-based lookups, connection pooling) often don't apply. Typical sessions: 5–50 files, <50 comments.
- **"Does the execution model make this possible?"** JavaScript is single-threaded — there are no race conditions between synchronous scope assignments and async fetches. Verify the threading model before claiming races.
- **"Realistic inputs?"** Markdown files can be 10,000+ lines (AI-generated plans) — perf concerns for large markdown are legitimate. Perf concerns for file lists or comment lists are not.
- **"Simpler than the duplication?"** For a single-file vanilla JS app and a flat Go CLI, inline code is often clearer than extracted helpers. Don't abstract for fewer than 3 call sites.

Project-specific calibration:

- **Unexport what isn't needed.** This is `package main` (a binary, not a library). If a function/type is only used within the package, it should be unexported.
- **Don't add `context.Context` to local git operations.** All git commands here are read-only local ops (diff, status, log, rev-parse). They complete in milliseconds and don't touch the network. The one path that benefits from context (`fileDiffUnifiedCtx` for lazy loading) already has it.
- **O(n) scans over file lists are fine.** A linear scan of `fileByPathLocked` is nanoseconds. Don't add map indices unless profiling shows a real bottleneck.
- **Mechanical duplication can be OK.** Comment CRUD (review vs file-scoped) is structurally identical but stable. Don't abstract stable boilerplate unless adding new operations would grow the duplication.
- **Don't fight browser built-ins.** `EventSource` auto-reconnects natively. `<details>`/`<summary>` handles keyboard natively. Don't reimplement.
</important>

<important if="you are about to claim work is complete — pre-completion checklist">

These issues recur in AI-generated code for this project. Only items NOT caught by automated tooling (golangci-lint, ESLint, Stylelint, axe-core) are listed.

### Go backend
1. Forgetting to clear review-level comments when clearing file comments
2. Missing fields in struct construction (silent data loss)
3. Creating wrapper functions that just delegate
4. Inline reimplementation of existing helper functions
5. Mixing `git status --porcelain` and `git diff --name-status` inconsistently
6. Not validating daemon health check response body

### Frontend JS
1. Missing `aria-label` on icon-only buttons (axe-core catches in E2E, but prevent at source)
2. Not checking `response.ok` after `fetch()`
3. Not resetting navigation state on context changes
4. SSE handlers re-fetching more data than changed
5. Async operations without dedup guards when triggerable from multiple sources
6. `.remove()` on elements with CSS exit animations (use animationend)

### Frontend CSS
1. Not defining new CSS variables in all 4 theme blocks
2. Leaving dead selectors after renaming CSS classes or DOM IDs
3. Referencing undefined CSS variables (check-css-vars.sh catches in pre-commit)
</important>

<important if="you are compacting context or summarizing this session">

Always preserve:
- The list of files modified in this session
- Any unresolved review comments or hunkyard feedback
- The current phase of any multi-phase workflow (review, ship, audit)
- Which worktree you're working in
</important>
