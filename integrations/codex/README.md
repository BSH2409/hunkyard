# Hunkyard — Codex CLI Integration

Drop-in configuration files that teach the OpenAI Codex CLI to use Hunkyard for reviewing plans and code changes.

## What's included

| Path | Install command | Purpose |
|------|----------------|---------|
| `skills/hunkyard/SKILL.md` | `hunkyard install codex` | `$hunkyard` skill — launches the interactive review loop |
| `skills/hunkyard-cli/SKILL.md` | `hunkyard install codex` | CLI reference — `hunkyard comment`, `hunkyard pull/push`, review file format |
| `plugin/hunkyard/.codex-plugin/plugin.json` | `hunkyard install codex-plugin` | Codex plugin manifest (skills + hooks) |
| `plugin/hunkyard/skills/*` | `hunkyard install codex-plugin` | Plugin-packaged copies of the skills |
| `plugin/hunkyard/hooks/hooks.json` | `hunkyard install codex-plugin` | `Stop` hook → `hunkyard plan-hook --mode codex` for proposed-plan review |

## Install

**Skills only** (on-demand `$hunkyard` when the target is already a file on disk):

```bash
hunkyard install codex              # project: .agents/skills/
cd ~ && hunkyard install codex      # global: ~/.agents/skills/
```

**Full plugin** (recommended — adds proposed-plan review in Plan mode):

```bash
hunkyard install codex-plugin              # project: plugins/hunkyard/ + .agents/skills/
cd ~ && hunkyard install codex-plugin      # global: ~/.codex/plugins/hunkyard/ + ~/.agents/skills/
```

The plugin install also:

1. Registers Hunkyard in `.agents/plugins/marketplace.json` (project) or `~/.agents/plugins/marketplace.json` (global)
2. Enables `hunkyard@local` in `~/.codex/config.toml`
3. Sets `features.plugins`, `features.hooks`, and `features.plugin_hooks` to `true` in that config

Safe to re-run. Existing files are skipped unless you pass `--force`.

## Plan mode and in-chat plans

In Codex Plan mode, the agent often keeps the plan in chat (`<proposed_plan>`) instead of writing a file. Bare `$hunkyard` needs a path, so it cannot review those plans on its own.

With `hunkyard install codex-plugin`, the `Stop` hook reads the proposed plan from the Codex transcript, writes it to a temp file, and runs `hunkyard plan-hook --mode codex`. You leave inline comments, the agent revises, and the turn does not complete until you approve.

Disable the hook: `export HUNKYARD_PLAN_REVIEW=off`

## Usage

Once installed:

- Type `$hunkyard` in Codex chat to review current git changes, a file, PR, or commit range
- In Plan mode with the plugin, proposed plans are reviewed automatically when the agent tries to finish the turn
- The `hunkyard-cli` skill teaches the agent about `hunkyard comment` and GitHub PR sync without manual invocation
