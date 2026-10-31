# Changelog

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
