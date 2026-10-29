# Hunkyard Integrations

Drop-in configuration files that teach your AI coding tool to use Hunkyard for reviewing plans and code changes.

This page is for you if you already use an AI coding agent and want it to hand its plans and diffs to Hunkyard for review. You need the `hunkyard` binary on your PATH first (see the [main README](../README.md)).

**Which do I need?**

- Claude Code or Codex, and you want plans reviewed automatically: use the plugin (the Claude Code plugin marketplace, or `hunkyard install codex-plugin`). Both add a plan-mode hook.
- Skills only: `hunkyard install claude-code` or `hunkyard install codex` adds the skills, and you ask for a review with `/hunkyard` (or `$hunkyard` in Codex).

## Quick install

```bash
hunkyard install <tool>     # Install for a specific tool in the current project
hunkyard install all        # Install for all supported tools
```

Safe to re-run. Existing files are skipped (use `--force` to overwrite).

**Global install**: run `cd ~ && hunkyard install <tool>` to install to your home directory. The integration is then available across all projects without per-project setup. Each tool reads from its own global path; `hunkyard install` routes the files to the right place automatically.

| Tool | Install command | Project destination | Global destination |
|------|----------------|---------------------|--------------------|
| Claude Code | `hunkyard install claude-code` | `.claude/skills/hunkyard/SKILL.md` + `hunkyard-cli` + `hunkyard-story` | `~/.claude/skills/hunkyard{,-cli,-story}/SKILL.md` |
| Codex | `hunkyard install codex` | `.agents/skills/hunkyard{,-cli,-story}/SKILL.md` | `~/.agents/skills/hunkyard{,-cli,-story}/SKILL.md` |
| Codex plugin | `hunkyard install codex-plugin` | loose skills + marketplace + `plugins/hunkyard/` (incl. hunkyard-story) | `~/.agents/` + `~/.codex/plugins/hunkyard/` |

## Plugin marketplace (Claude Code)

For the full experience, install via the plugin marketplace. This gives you:
- A `/hunkyard` slash command for the review loop
- A model-discoverable `hunkyard-cli` skill for review files, `hunkyard comment`, `hunkyard pull/push`, etc.
- A wording-gated `/hunkyard-story` skill for chaptered diff overviews that then continues the `/hunkyard` review loop

```
claude plugin marketplace add BSH2409/hunkyard
claude plugin install hunkyard@hunkyard
```

The marketplace manifest lives at the repo root (`.claude-plugin/marketplace.json`) and points to the plugin files in `integrations/claude-code/`.

### `hunkyard install` vs plugin marketplace

| | `hunkyard install claude-code` | Plugin marketplace |
|---|---|---|
| **Scope** | Per-project (committed to repo), or user-wide with `cd ~ && hunkyard install claude-code` | User-wide |
| **What's installed** | `/hunkyard`, `hunkyard-cli` and `hunkyard-story` skills | `/hunkyard`, `hunkyard-cli` and `/hunkyard-story`, plus the plan-mode review hook |
| **Plan mode** | Ask for the review yourself with `/hunkyard` | Hook intercepts `ExitPlanMode` and opens Hunkyard for review automatically |
| **Good for** | Teams - everyone gets the integration when they clone | Individual users - works across all projects |
| **Setup** | Run once per project | Install once, works everywhere |

Both give you the `/hunkyard` review loop and the `hunkyard-cli` skill, which teaches the agent `hunkyard comment`, the review file format, `hunkyard pull/push` and resolving comments without opening the browser loop. Only the plugin marketplace adds the plan-mode hook.

## Claude Code plan approval mode

The Claude Code plugin intercepts `ExitPlanMode` with a narrowly matched
`PermissionRequest` hook. By default, approving in Hunkyard allows the plan exit and
leaves Claude Code to restore its existing permission mode. To choose the mode
deterministically, set `plan_approve_mode` in your global Hunkyard config:

```json
{
  "plan_approve_mode": "acceptEdits"
}
```

Disable automatic plan review per shell or globally with
`export HUNKYARD_PLAN_REVIEW=off`. This only disables the plan-exit hook; manually
running `hunkyard plan` still opens a review.

Supported values are `default`, `manual`, `acceptEdits`, `plan`, `auto`,
`dontAsk`, and `bypassPermissions`. The `manual` alias requires Claude Code
2.1.200 or newer. On approval, Hunkyard returns Claude Code's documented
`decision.updatedPermissions` entry:

```json
{
  "type": "setMode",
  "mode": "acceptEdits",
  "destination": "session"
}
```

`destination: "session"` keeps the change in memory for the current Claude Code
session only. The setting is global-only (`~/.hunkyard.config.json`); a repository's
`.hunkyard.config.json` cannot change your permission policy. Unset preserves the
default hook behavior, and invalid values are ignored with a warning.

The hook approval and mode switch do not override matching deny or ask rules.
Claude Code can also disable `auto` through `permissions.disableAutoMode`.
`bypassPermissions` is intentionally dangerous and should only be used in an
isolated environment. Claude Code applies it only when the session started with
bypass mode available (for example `--allow-dangerously-skip-permissions` or
`--dangerously-skip-permissions`) and managed settings have not disabled it;
otherwise Claude Code treats the update as a no-op.

## Codex plugin

For the full Codex experience, install the plugin. This gives you:

- A `$hunkyard` skill for the review loop (plus loose copies under `.agents/skills/` so bare `$hunkyard` works even outside the plugin)
- A `hunkyard-cli` skill that auto-activates when working with review files, `hunkyard comment`, `hunkyard pull/push`, etc.
- A **proposed-plan review hook** - intercepts Codex's `Stop` hook when the agent proposes a plan in Plan mode, writes it to disk, and opens Hunkyard for inline review before the turn ends

```bash
cd ~ && hunkyard install codex-plugin    # global (recommended)
hunkyard install codex-plugin            # per-project (commit plugins/hunkyard/ for the whole team)
```

`hunkyard install codex-plugin` registers the plugin in a local Codex marketplace (`.agents/plugins/marketplace.json` or `~/.agents/plugins/marketplace.json`), copies plugin files to `plugins/hunkyard/` (project) or `~/.codex/plugins/hunkyard/` (global), enables the plugin in `~/.codex/config.toml`, and turns on `features.plugins`, `features.hooks`, and `features.plugin_hooks`.

Plugin source files live in `integrations/codex/plugin/hunkyard/`. See [`integrations/codex/README.md`](./codex/README.md) for layout and manual setup.

### `hunkyard install codex` vs `hunkyard install codex-plugin`

| | `hunkyard install codex` | `hunkyard install codex-plugin` |
|---|---|---|
| **Scope** | Skills only (project or global) | Skills + Codex plugin + plan hook |
| **What's installed** | `$hunkyard` and `hunkyard-cli` skills under `.agents/skills/` | Same skills, plus `plugins/hunkyard/` (or `~/.codex/plugins/hunkyard/`) with bundled skills and a `Stop` hook |
| **Plan mode** | Agent must write the plan to a file before `$hunkyard` works | Hook captures in-chat proposed plans (`<proposed_plan>`) and reviews them automatically |

Both approaches give you `$hunkyard` and the `hunkyard-cli` skill. Only `codex-plugin` adds the proposed-plan hook - without it, typing `$hunkyard` on an in-chat plan (e.g. after choosing "No and stay in Plan Mode") does nothing useful because there is no file path for `hunkyard` to open.

Disable automatic plan review per shell or globally with
`export HUNKYARD_PLAN_REVIEW=off`. Manual `hunkyard plan` invocations are unaffected.

## Invocation policy

The interactive `hunkyard` review cycle is wording-gated, not hard-disabled.
Skill descriptions and workflow docs tell agents to launch Hunkyard only when the
user invokes the platform command (`/hunkyard` in Claude Code, `$hunkyard` in Codex)
or directly asks to use Hunkyard. A normal
request to review code, a plan, a diff, a PR, or a page does not count.

`hunkyard-cli` is intentionally model-discoverable. It teaches agents how to leave
and reply to Hunkyard comments, interpret review JSON, and synchronize
GitHub PR feedback, but it does not start the interactive review cycle.

`hunkyard-story` is wording-gated like `/hunkyard`: agents author a chaptered story
overview only when the user invokes `/hunkyard-story` (or the tool equivalent) or
directly asks to generate a hunkyard story. After ingest they reconnect with bare
`hunkyard` and continue the same wait → address → next-round cycle. It does not run
as part of a normal review request.

The only automatic interactive path is a lifecycle hook immediately after
planning mode. The Claude Code plugin and Codex plugin
retain their existing plan-exit hooks.

## What these do

All integrations follow the same pattern:

1. **Pick the review target** - current git changes by default, or an explicit file, plan, PR, or commit range when the user names one
2. **Launch Hunkyard** - the agent runs the matching `hunkyard` command to open the review in your browser
3. **Address feedback** - after review, the agent reads the review file to find your inline comments and revises the target
4. **Continue the review loop** - the agent reruns the printed next-round command until you finish with no unresolved comments

Each integration also teaches the agent about:
- **`hunkyard comment`** - leave inline review comments programmatically without opening the browser
- **review file format** - how to read comments, resolve them with threaded replies
- **`hunkyard pull/push`** - sync reviews with GitHub PRs (push supports `--event approve|request-changes|comment`)

## Next steps

- Something here out of date? [Open an issue](https://github.com/BSH2409/hunkyard/issues). Include the tool, its version and the command you ran.
