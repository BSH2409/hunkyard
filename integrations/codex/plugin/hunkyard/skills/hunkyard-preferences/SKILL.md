---
name: hunkyard-preferences
description: Use when the user corrects coding behavior, states a reusable convention, reviews changes, asks to inspect or retire preferences, or needs preference hooks and local maintenance. Uses Hunkyard's Go engine in Codex or Claude CLI.
---

# Coding preferences

Use `hunkyard preferences` to capture and apply reusable personal coding guidance. The canonical store is `${HUNKYARD_PREFERENCES_HOME:-$HOME/.agents}`, shared by Codex and Claude. The engine runs locally and needs no Ruby runtime.

Before coding, load relevant generated guidance with `hunkyard preferences select --query 'task description' --cwd "$PWD" --json`. Read the returned `text` as defaults: current user instructions and repository standards win, repository preferences override global preferences, and personal preferences override imported source defaults. Missing guidance is valid. Never read raw observations, lessons, or transcripts to load a persona.

When the user gives feedback:

1. Separate the requested edit from reusable guidance. A one-off rename or factual correction does not establish a lasting preference. Agent prose, quotations, and neighboring context describe the corrected action; only the user's own feedback establishes intent.
2. Ignore purely task-specific edits. Capture an ambiguous reusable correction as a candidate. For a clear durable instruction, write a short imperative describing the action and its trigger or exception. Preserve useful specifics and conditions; avoid universal slogans.
3. Use global scope for a personal convention across repositories and repository scope for a local convention. Supported surfaces are api, architecture, communication, database, devops, frontend, jobs, naming, performance, rails, reviews, ruby, security, services, testing, wordpress.
4. Keep secrets, customer identifiers, pasted code, patches, and long examples out of stored directives. Send feedback through stdin using the host's structured execution tool or a private temporary file; never interpolate untrusted text into shell code.

Candidate capture: `hunkyard preferences capture --cwd "$PWD" --json`, with feedback on stdin.

Explicit capture: `hunkyard preferences capture --explicit --directive 'Prefer names that describe domain behavior.' --scope global --surface naming --json`, with the user's feedback on stdin. Use `--scope repository --cwd "$PWD"` for a repository convention. Do not promote an uncertain correction merely to make it appear in a persona.

For browser review batches, follow Hunkyard's review workflow and classify **every human revision** through `hunkyard feedback learn`. That command preserves the publication identity, prevents duplicate evidence, and validates completion before acknowledgment. Do not also capture the same batch through this CLI. Agent replies never count as human evidence.

Some preferences are enforced at action time by guards. When a `git commit` or `git push` is blocked, read the reason, change the action as it says, and if the user still wants it, ask them to run `! hunkyard preferences allow RULE_ID --once` themselves. Never run `preferences allow` yourself. When the user asks to enforce a rule, add a guard with `guard add --kind KIND --rule RULE_ID [--pattern P]... --json` (kinds: staged_paths, no_coauthor_trailer, commit_message_format, no_unrequested_commit, diff_check). A `diff_check` guard names one check as its pattern (ruby_single_line_def, ruby_semicolon_statement, js_console_log, todo_without_owner, skipped_test_added); besides blocking at Stop, it posts rule-check comments in Hunkyard reviews, and the reviewer's Confirm or Dismiss is recorded as rule evidence. Add `--match` and `--not-match` examples so a guard proves what it blocks and allows; `guard test --json` re-runs them, and `check --cwd DIR -- COMMAND` reports whether a command would be blocked without side effects. Use `doctor --json` to check hooks and maintenance.

Teams share conventions as **team packs**: a folder with `SKILL.md` whose `## References` section links `reference/<surface>.md` files, one `- directive` bullet per rule, plus `pack.json` (name, version, digest). `pack export --name NAME --version V --out DIR --json` writes your global rules as a pack (`--scope repository --cwd DIR` for this repository's rules); `pack import DIR --json` imports someone else's (`--dry-run` first; `--scope repository --cwd DIR` to apply it to one repository); `pack list --json` and `pack remove NAME --json` manage them. A repository can commit its own pack under `.hunkyard/preferences-pack/`; session start imports it for that repository and removes its rules when the folder goes away; `pack remove repo-…` declines it until its content changes. Team-pack rules are removed with their pack, not retired one by one. `export --scope repository --cwd DIR --to FILE` writes this repository's rules as one markdown file. Pack rules rank below personal rules and above source defaults, are never edited by maintenance, and only add directives: a pack cannot change guards, hooks or configuration.

Use `search --query TEXT --surface NAME --json` to inspect safe rule summaries, `retire RULE_ID --json` to stop applying a personal rule, `build --json` to regenerate personas, and `audit --duplicates --json` to check integrity. Retiring preserves evidence. When the user wants to browse rules, see why rules were loaded, or check health visually, point them to the **Preferences** tab in a Hunkyard review's Settings. Do not hand-edit generated files or canonical records.

Read [local maintenance and installation](references/local-maintenance.md) when installing hooks, consolidating candidates, importing source defaults, or migrating legacy lessons. Maintenance proposals come from this host session, or from the Claude or Codex CLI the user configured for automatic maintenance.
