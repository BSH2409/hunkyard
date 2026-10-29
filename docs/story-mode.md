# Story mode

Story mode adds an optional narrative layer to a diff review. Instead of
showing only a flat file list, Hunkyard can show a prologue and ordered chapters
that group changed hunks by theme: routing, persistence, tests, generated
files, and so on.

It is an explainer, not a reviewer. A story should help a human understand the
shape of a change before reading the diff. It should not add review comments,
judge the patch, or suggest fixes.

## When to use it

Use story mode for branch, PR, or range reviews where the diff is large
enough that a thematic overview helps. It is diff-scoped only - not for
positional file reviews, live, preview, or plan reviews.

### Recommended: `/hunkyard-story` skill

After `hunkyard install <tool>`, invoke the story skill explicitly (for example
`/hunkyard-story`, `$hunkyard-story`, or `/skill:hunkyard-story`). The agent:

1. Runs `hunkyard story --guide` and `hunkyard story --prep <path>`
2. Authors `prologue` / `chapters` / `support` JSON
3. Ingests with `hunkyard story --story-file <path>` and opens the story view
4. Runs bare `hunkyard` to wait for Finish Review, then addresses comments on
   source files and loops rounds - the same cycle as `/hunkyard`

Agents must not infer story generation from a generic review or `/hunkyard`
request - only from an explicit story invoke or a direct ask to generate a
hunkyard story.

### Alternative: `hunkyard story` + `agent_cmd`

From the terminal (no skill):

```bash
hunkyard story
hunkyard story --pr 123
hunkyard story --range main..HEAD
```

This uses your global `agent_cmd` to author the story (separate LLM spend),
saves it into the existing review JSON, starts or updates the review daemon,
and opens the browser at the story view.

```json
{
  "agent_cmd": "claude -p"
}
```

`agent_cmd` must be an agentic CLI that can read files from disk. Story
generation is prompt-by-reference: Hunkyard writes the full prep file to disk and
tells the agent to read it. The diff is not pasted into the prompt.

## Token cost

Story generation is **LLM-driven exploration**: the agent reads the prep file,
may open related source for context, and writes the chapter JSON. That uses
your agent's tokens (in-session with `/hunkyard-story`, or a separate spawn with
`agent_cmd`). Hunkyard itself does not bill for stories.

Spend depends more on **how complex and multi-theme the change is** - and how
much the model explores - than on raw file count or diff size. Cost does
**not** scale linearly with files or lines changed. Tiny diffs are cheap;
large multi-theme PRs cost more, but two big diffs can land in a similar
ballpark if exploration depth is similar.

In our experience, complex PRs (~20–50 files, ~2k–5k lines changed) cost about
**$1–$1.40** with Claude Opus 5 via `/hunkyard-story`.

## Common commands

| Command | Use |
| ------- | --- |
| `hunkyard story` | Generate a story for the current branch diff |
| `hunkyard story --refresh` | Regenerate an existing story |
| `hunkyard story --no-spend` | Reopen an existing story without calling `agent_cmd` |
| `hunkyard story --clear` | Remove the story and return to the flat diff view |
| `hunkyard story --no-open` | Save or resume without opening a browser tab |
| `hunkyard story --skip-llm` | Save a support-only stub story for testing the renderer |
| `hunkyard story --guide` | Print the resolved authoring guide and JSON shape |
| `hunkyard story --prep /tmp/hunkyard-story-prep.txt` | Write the full prep file for manual authoring |
| `hunkyard story --story-file /tmp/hunkyard-story.json` | Ingest pre-authored story JSON |

If a story already exists, `hunkyard story` reopens it. Use `--refresh` to replace
it or `--clear` to remove it.

## What Hunkyard saves

The story is stored on the existing review file under `story`:

```text
~/.hunkyard/reviews/<session-key>/review.json
```

There is no separate story cache or sidecar file in v1. Because story data
lives in the review JSON, it follows the same per-branch or per-range session
key as the rest of the review.

Agents should emit only the editable story fields:

```json
{
  "prologue": {
    "title": "What changed",
    "overview": "A short overview of the shape of the diff.",
    "motivation": "Optional context for why these changes exist.",
    "key_changes": ["Optional concise bullets"],
    "risks": ["Optional concise bullets"],
    "diagram": ""
  },
  "chapters": [
    {
      "id": "ch1",
      "title": "Request routing",
      "summary": "Routes now carry the new story endpoint and state.",
      "hunk_refs": [
        {"file_path": "internal/server/prompts.go", "old_start": 42}
      ],
      "diagram": ""
    }
  ],
  "support": [
    {
      "hunk_refs": [
        {"file_path": "go.sum", "old_start": 120}
      ],
      "reason": "Dependency checksum churn."
    }
  ]
}
```

Hunkyard fills `version`, `generated_at`, `base_sha`, `head_sha`,
`scope_fingerprint`, and `coverage` during ingest. Do not include them in
agent-authored JSON.

## Author a story manually

Manual authoring uses the same validation path as LLM generation:

```bash
hunkyard story --guide > /tmp/hunkyard-story-guide.md
hunkyard story --prep /tmp/hunkyard-story-prep.txt
```

Read both files, then write a JSON object with `prologue`, `chapters`, and
`support` to a temp file:

```bash
hunkyard story --story-file /tmp/hunkyard-story.json
```

On every ingest attempt, Hunkyard prints a coverage report to stdout:

```json
{"ok":true,"indexed":12,"placed":12}
```

Exit code `0` means the story was saved. Exit code `1` means nothing was
saved, usually because the JSON did not parse, a hunk was duplicated, fewer
than half of the hunks were placed, or the diff changed since the prep file was
written.

If `auto_repaired` is true, Hunkyard saved the story but back-filled omitted hunks
into `support[]`:

```json
{"ok":false,"indexed":12,"placed":10,"missing":["(web/app.js, 380)"],"auto_repaired":true}
```

That is acceptable, but usually worth re-authoring if the missing hunk belongs
in a real chapter.

## Write good chapters

A useful story follows a few constraints:

- Group by theme, not by file. Cross-file chapters are expected.
- Keep chapter titles short. Hunkyard truncates titles beyond 48 characters.
- Use chapter array order as the reading order. There is no separate `order`
  field.
- Put generated files, lockfiles, vendored data, and other mechanical changes
  in `support[]`.
- Reference hunks only by the `(file_path, old_start)` pairs from the prep
  file.
- Leave `diagram` empty unless a Mermaid diagram clarifies a real structure or
  flow.
- Prefer 2-6 chapters for a typical PR. One chapter is fine when the change is
  genuinely one theme.

## Make your own story prompt

Story generation uses the `on_story_generate` prompt hook. The stock prompt is
shipped at:

```text
integrations/prompts/on_story_generate.md
```

Install a copy to edit:

```bash
hunkyard install story-prompts
```

Run that from a repo root to create a project prompt:

```text
.hunkyard/prompts/on_story_generate.md
```

Run it from your home directory to create a global prompt:

```text
~/.hunkyard/prompts/on_story_generate.md
```

You can also point config at a custom file:

```json
{
  "prompts": {
    "on_story_generate": "file:.hunkyard/prompts/my-story-guide.md"
  }
}
```

`on_story_generate` has no `:mode` suffix because story mode only applies to
diff scopes. Resolution order is:

1. Project `.hunkyard.config.json` `prompts.on_story_generate`
2. Global `~/.hunkyard.config.json` `prompts.on_story_generate`
3. Project `.hunkyard/prompts/on_story_generate.md`
4. Global `~/.hunkyard/prompts/on_story_generate.md`
5. Stock Hunkyard template

Overrides replace the entire prompt. If you customize
`on_story_generate`, keep the operational requirements in your prompt:

- Tell the agent to read `{{.prep_path}}`.
- Tell it to emit raw JSON only.
- Tell it to emit only `prologue`, `chapters`, and `support`.
- Include `{{.story_schema_json}}` or an equivalent schema.
- Preserve the hunk coverage rule: every hunk appears exactly once in
  `chapters[]` or `support[]`.
- Tell the agent not to call `hunkyard comment` or `hunkyard push`.

Useful template variables:

| Variable | Meaning |
| -------- | ------- |
| `{{.prep_path}}` | Path to the full prep file the agent must read |
| `{{.story_schema_json}}` | JSON shape Hunkyard can ingest |
| `{{.commit_messages}}` | Commit messages over the diff scope |
| `{{.diff_scope_kind}}` | `committed` or `workingTree` |
| `{{.base_sha}}` / `{{.head_sha}}` / `{{.merge_base_sha}}` | Scope SHAs when available |
| `{{.pr_number}}` / `{{.pr_url}}` / `{{.pr_title}}` / `{{.pr_body}}` | PR context when available |
| `{{.session_key}}` | Review session key |
| `{{.review_path}}` | Path to the review JSON file |

Project-level story prompts use the same project prompt trust flow as other
agent prompts. If a project prompt is untrusted, `hunkyard story` will refuse to
use it until you trust project prompts from the Hunkyard UI or choose to use Hunkyard
defaults.

See [Agent prompts](agent-prompts.md) for the full prompt hook reference.
