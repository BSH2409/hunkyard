# Changelog

## v1.0.3 — unreleased

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
- The preference store folder is now `hunkyard-preferences/` (under `${HUNKYARD_PREFERENCES_HOME:-~/.agents}`). The first Hunkyard command moves an existing `learn-preferences/` folder there, keeping every rule, observation and generation. `LEARN_PREFERENCES_HOME` still works as a fallback, and generated files with the old header stay replaceable. `hunkyard preferences doctor` warns if an older Hunkyard still running writes to the old folder after the move.
- Hook notes name Hunkyard as the one doing the work, for example "Hunkyard took the hunk hint…".
- The status line reads `◆ Hunkyard: …` instead of `◆ yard: …`.

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
