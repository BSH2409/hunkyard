# Changelog

## v1.0.4 — 2026-11-02

### Agent decisions

- `hunkyard ask --session ID --option A --option B "Which one?"` puts a question to the reviewer. It appears in a **Questions from the agent** dock at the bottom right of the review, one question per page, with your options or a written answer, a review page that lists every question and answer, and **Submit answers**. The answers come back in the next feedback batch as `decisions`; a blocking question keeps Approve disabled until it is answered.

### Tool results in the review

- `hunkyard annotate --format coverprofile|lcov|sarif|go-test-json <file|->` imports test, coverage and static-analysis output into the open review. Failures and findings appear as comments marked as tool output (filter: **Tools**), uncovered lines are tinted in the gutter, and a file header shows its coverage. Running it again replaces that tool's earlier results. Tool output is never counted as your preference evidence.
- Preference rules bound to a diff check (for example "no `console.log`", "every TODO has an owner") run over the diff when a review opens. Each hit is a finding you can **Confirm** or **Dismiss**; the verdicts become evidence for the rule.

### Reading a change

- **Impact.** Each changed file shows the callers and tests of the symbols its diff defines or removes (Git reviews; a textual search of tracked files, capped).
- **Overview and risk order.** A Git review opens with an overview (branch, base to head, commits, file and line totals), and the file list can be ordered by an explainable risk score (auth, secrets, migrations and your own `risk_patterns`).
- **Image diff.** A changed PNG, JPEG, GIF or WebP shows its base and head side by side, with a slider and an onion-skin mode.

### Across reviews

- `hunkyard inbox` and the `/inbox` page list every running and paused review on the machine with its round, open comments and agent state, and resume a paused one.
- `hunkyard stats` and the Preferences tab show how your reviews went over a window: rounds and time to approve, comments per round, how comments ended (resolved, open, claimed fixed but unchanged), and which rules you confirmed or dismissed.
- **Preference packs.** A repository commits conventions in `.hunkyard/preferences-pack/` and every session in it loads them. `hunkyard preferences pack export`, `import`, `list` and `remove` share a pack with a teammate; repository rules can be exported to seed one. A pack installs once per name: importing it for another repository or scope is refused until you remove the first.

### Security

- Live mode no longer trusts the menu options a reviewed page sends. A page could include markup in them and run script in the review page, which could read or wipe the review.
- The live proxy checks the `Host` header like the review server (DNS rebinding). A request another site sends through it reaches your app as the browser sent it, without the configured session cookies, so your app's own CSRF check decides.
- **A preview page now has its own port** (the review port plus one, as a live proxy already does) and cannot reach the review page's API. Docker users must publish that port. Files beside the previewed page are served only if they stay inside its folder, including through symlinks. HTML and SVG served from the repository by the review server are sandboxed.
- A project's prompt files must stay inside the project; before, a repository could name any file.

### Fixes

- **Feedback the agent missed.** After the first round, a comment that was reopened because the agent's reply did not change the code, or whose label or blocking flag you changed, was not sent again. It is now.
- **Comments and replies.** `hunkyard comment --selector`, bulk replies, renames, scope changes and `hunkyard story` no longer overwrite a comment added at the same moment. A comment added during a focus change, or in the last moments before the agent finishes a round, is kept.
- **Rounds.** In files mode, comments follow their lines when the agent edits a file and finishes before the next poll, and a round with no edits no longer moves them.
- **Resume.** The stale-review sweep that runs when you start a review can no longer delete the review you are resuming. Live, preview and range reviews resume as what they were (a range keeps its pinned commits), and live reviews keep their questions and overall comments.
- **Sessions.** Live and preview reviews belong to the Claude, Codex or terminal session that started them, as other reviews do. `feedback await --follow` finds paused plan and `--output` reviews and waits out a resumed daemon's start-up; `hunkyard stop` clears paused reviews.
- **Publishing** works for a file outside the repository and on a plain-HTTP LAN address, a pin screenshot that a published batch points to is kept when the pin moves, and replies you write under a tool finding count as your feedback.
- **Preferences.** A rule you restate replaces an imported or retired copy; maintenance retries on the current batch; a guard no longer lets `git add . && git commit` through, applies by the repository the command runs in, and spends your one-time override only when the command passes; "open a PR and merge" counts as asking to commit, "do not create a PR" does not.
- **Git and diffs.** A file replaced by a symlink, a rename when `diff.renames=false`, copy detection, merge and root commits, and renamed or non-ASCII paths in line counts all show correctly. `go test -json`, coverage and lcov imports no longer mix packages or mark covered lines uncovered.
- **Review page.** A selection that crosses removed and added lines gives the line you end on; Ctrl+Enter cannot post twice or save an unfinished image upload; edits and open replies survive a refresh or scope change; open reply and edit boxes survive a live-mode reload; drafts are kept per review; pins on elements with unusual ids and on SVG resolve; the story view scrolls on phones.
- Smaller: IPv6 addresses in review URLs, disabling two shortcuts, a folder named `constructor` in the file tree, a stuck daemon no longer hangs `hunkyard story`, `hunkyard annotate` works from a subdirectory.

## v1.0.3 — 2026-11-01

### Speed on large repositories

- Opening a review, picking a commit, picking **Working changes** and changing the base share one cached reading of the working tree. It is keyed on HEAD, the index and the base, and concurrent requests wait for one reading instead of each running git.
- Diffs and line counts for many files come from one `git diff` (and one `--numstat`) instead of one call per file.
- Commits and commit ranges are cached by SHA, so returning to a commit reuses its diff.
- Untracked and filtered files are listed in one pass; remote branches are listed without a per-ref ambiguity check.
- File names with spaces, quotes or non-ASCII characters are parsed correctly (`-z` output throughout).
- A base or commit change reloads the review once, without blanking the file list, and a quick second pick wins over the first.
- `hunkyard check` suggests `git config core.untrackedCache true` when `git status` takes more than 500 ms.

### Idle pause

- A review with no activity for 30 minutes pauses: the daemon exits and frees its port, and open tabs show **Review paused**. Page interaction, review API calls and agent turns (through the preference hooks) count as activity.
- `hunkyard --session REVIEW_ID` resumes on the same port when it is free, and the open tab reconnects by itself. Reviews with no comments resume too.
- `feedback await --follow` waits through a pause; a one-shot await reports `{"status":"paused"}`. `hunkyard status` and `hunkyard resume --list` show paused reviews, and `hunkyard stop` clears them.
- Global config `idle_pause_minutes` (or `HUNKYARD_IDLE_PAUSE_MINUTES`) sets the idle time; `0` disables it.

### Refresh banner

- The page no longer redraws when files change underneath it. A **This review has updates** banner offers **Refresh** instead. Base and commit changes always load the latest state, and agent rounds stay automatic.

### Preferences

- The `learn-preferences` skill is now `hunkyard-preferences`. `hunkyard install preferences` removes the old copy when it is unmodified.
- The preference store folder is now `hunkyard-preferences/` (under `${HUNKYARD_PREFERENCES_HOME:-~/.agents}`). The first Hunkyard command moves an existing `learn-preferences/` folder there, keeping every rule, observation and generation. The move also rewrites the generated-file headers, so builds keep working. `LEARN_PREFERENCES_HOME` is no longer read; set `HUNKYARD_PREFERENCES_HOME` instead. `hunkyard preferences doctor` warns if an older Hunkyard still running writes to the old folder after the move.
- Hook notes name Hunkyard as the one doing the work, for example "Hunkyard took the hunk hint…".
- The status line reads `◆ Hunkyard: …` instead of `◆ yard: …`.

### Security

- `commit` query parameters must be a SHA or a SHA range. Before, a value starting with `-` reached git as an option, so a web page could truncate files through `GET /api/session` (for example `commit=--output=<file>`).
- Comment, file and diff endpoints refuse paths outside the repository, including `..` and symlinks that leave it. Before, posting a comment on `../file` read that file into the review.
- The DNS-rebinding check now applies on the default `0.0.0.0` bind too: requests must name `localhost`, an IP address, the listen host or `--public-url`.

### Fixes

- `hunkyard comment` and the open review no longer lose each other's comments, replies or resolves when they write at the same moment (a shared review lock and a three-way merge).
- A finish hook that times out is stopped with its child processes, instead of blocking finish until it ends.
- A second review starts on a free port when the default 5000 is taken; a port you set is kept.
- Markdown round diffs trim unchanged lines first: a small edit to a 10,000-line plan diffs in milliseconds.
- A failed comment delete keeps the comment and says so; "Resolve all" stops and reports any comment it could not resolve instead of finishing.
- CLI errors no longer print a stray `exit` line, and `hunkyard cleanup --days` without a number is an error.
- A request that arrives as a review pauses is refused instead of being lost; a burst of comment updates no longer crowds out the pause or shutdown notice; comment refreshes never overlap, so a slow one cannot show older comments.
- A working-tree change seen while the tree was being read is no longer forgotten; the waiting screen counts the agent's edits every second again.
- The Preferences tab always shows the last view you picked, a one-time guard override that could not be removed is not used, and a paused review with no comments is cleaned up like any other old review.

### Live mode and HTML preview

- `hunkyard live http://localhost:3000` reviews a running app on this machine and `hunkyard preview page.html` a local HTML file; pin comments to elements on the page. Only loopback origins are accepted, and nothing in the reviewed app or repository changes.
- **Pin screenshots.** Each pin captures the whole page in the frame (SnapDOM, bundled) with the element boxed in red with a white halo. Apps that scroll inside a container are captured in full. When capture fails, the pin says why.
- **What the agent gets.** Published pins carry a `pin` block: route, selector, status, the screenshot URL on the review port, the box's exact rectangle, the marker style and the viewport.
- **Outdated pins.** Each pin records a fingerprint of its element. When the element changes or disappears, the pin shows **Outdated** and the agent gets `outdated: true`; a page not opened since the last round is `unchecked`. Re-anchoring a pin takes a new screenshot. Older pins marked `drifted` load as outdated.
- Commenting again on exactly the same element (same route, selector and tag chain) adds a reply to its open pin instead of a second pin.
- An app that redirects to another site shows an **Open in real browser** notice instead of failing.
- A reviewed page loads its own outside resources (web fonts, captcha scripts, images) as it does when opened directly, so it looks the same in the frame and in pin screenshots. Hunkyard itself still connects to nothing outside this machine. `HUNKYARD_BLOCK_EXTERNAL=1` keeps reviewed pages to local resources.
- A review opened by a LAN or container address can show its live page; the frame was allowed only on `localhost`.
- `hunkyard live` starts with a warning when the app answers slowly (for example while it compiles assets) instead of refusing; only a refused connection stops it.
- Pin markers sit on their element on any page layout: the marker layer is fixed to the viewport and follows the element through window, body and panel scrolling, resizes and re-renders. A marker hides while its element is scrolled out of its panel, covered by a panel, menu or modal, or not rendered yet, and comes back when the element is visible.
- In Comment mode markers are dimmed and clicks pass through them to the element underneath; in Browse they open their comment.
- A pin whose selector no longer matches is found again only when exactly one element has its role and name; it is never moved onto a lookalike.
- Screenshots of panels that scroll sideways capture their full width with the box on the element; captures on 2x screens are taken at 1x with the box in place.
- The composer names the picked element with spaces between its parts (table cells, lines).
- The page proxy (review port + 1) listens on the same host as the review, so in Docker the published proxy port reaches it; `--host 127.0.0.1` keeps both on loopback.
- Attachments (pin screenshots and pasted images) are never served to another site, including the reviewed app's own port, and are sent `private, no-store`. Pin screenshots are deleted with their pin, and on approval even when `cleanup_on_approve` is off.

### Removed

- Sapling and Jujutsu support. Git is the only backend; a `vcs` config value other than `git` prints a warning and is ignored.

### Preferences tab

- The Yard is now the **Preferences** tab in a review's Settings. The standalone `hunkyard yard` command and its `/yard` page are removed, and its API moved from `/api/yard/*` to `/api/preferences/*`.
- **Pipeline** lists the candidate, active and retired rules in their columns, trims long captured text (full text on hover), and carries **Run maintenance** while observations or candidates are waiting.
- **Persona** files render as markdown with distinct heading sizes, and the file list stays in view while a file scrolls; **Activity** is a timeline with a marker per event.
- Settings widens for this tab. Its views are real tabs (arrow keys, Home and End) in a bar pinned above the content, together with the view's tools.
- A view that takes a moment keeps the previous one dimmed and inert instead of showing it under the new tab; views already loaded switch instantly.
- **Rules** shows how many rules match, with search and status, scope and surface filters, loads 50 at a time, and asks before it retires a rule (Escape backs out).
- **Run maintenance** sits at the right of the Pipeline bar as "Consolidate N captured · M candidates", says when there is nothing to do, and reports that it started.
- Load failures are announced with a Retry button. Health counts warnings in the tab badge, and a guard with no examples shows as unproven instead of passing.
- "Make it a rule" can add a rule for this repository as well as for every repository, and asks for its surfaces instead of assuming one.

## v1.0.2 — 2026-10-31

### Preference Yard

- `hunkyard yard` opens the Preference Yard. It listens on `127.0.0.1` unless `--host`, `HUNKYARD_HOST` or the global config `host` says otherwise. The same Yard is also a tab in a review's Settings. It shows:
  - **Rules**, with evidence and violation counts. You can retire a rule, promote a repository rule to global, or add a rule.
  - **Pipeline**: captured observations waiting for maintenance.
  - **Persona**: the generated persona files.
  - **Loads**: which rules each recent session received, and why.
  - **Activity** and **Health**, with a button to run maintenance.

### Review UI

- Settings and keyboard shortcuts are split into General and Review sections.
- Comment labels: nit, issue, suggestion, praise and question, with a **blocking** flag. Approve is refused while any blocking comment is unresolved, both in the dialog and on the server.
- **Claimed fixed, code unchanged.** If the agent says it fixed a comment but the commented lines are identical, the comment reopens next round with a badge, and the published batch sets `claim_unchanged`.
- **Since last review.** In Git mode, a toggle shows only what changed after your last published review.
- **Changed since you viewed.** A viewed file that changes comes back un-viewed, with a badge. This includes files loaded on demand in large reviews.
- **Reviewer edits.** Lines you edit on disk during a review are published as `edit` comments, so the agent keeps them and can learn from them. This is skipped under range focus and in rounds with agent activity.
- The file count shows as a badge beside the Files title.

### Learned preferences

- The preference store moved to `${LEARN_PREFERENCES_HOME:-~/.agents}` and is shared with the `learn-preferences` skill.
- Hooks deliver published review feedback without polling. On Claude, a background `asyncRewake` handler wakes the session when you publish.
- **Guards:**
  - Each guard carries `--match` and `--not-match` examples, which are checked when it is added.
  - `guard test` re-runs every guard's examples.
  - `preferences check -- CMD` reports whether a command would be blocked, without side effects.
- Editing a file loads the rules for its area once per session. The first prompt after an interrupt is captured as a correction.
- Automatic maintenance uses a configurable host, model and effort: Claude Opus 5.5 high or Codex 6.1 Sol high by default. Every outcome is recorded in an activity log and shown by `doctor`.

### Fixes

- The update notice handles versions correctly, and the Nix vendor hash is fixed.
- Owner records and reviews of deleted paths work on Windows.
- Untracked new files are checked at Stop.

### Known limitations

- Learned preferences and the Preference Yard are not supported on native Windows yet; use WSL (#11). Reviews work on Windows. There, `feedback learn` records decisions as `unsupported`, installed hooks stay silent, and the Yard explains why it is unavailable.
- Four quote-highlight E2E tests fail on Windows Chromium (#12).
