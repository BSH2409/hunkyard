# Markdown rendering and GitHub parity

Hunkyard renders Markdown documents to look the way GitHub renders a `.md`
file in a repository. Parsing is markdown-it (CommonMark plus GFM tables and
`~~strikethrough~~`), extended by `web/hunkyard-gfm.js` with the rest of GitHub
Flavored Markdown. `web/hunkyard-line-blocks.js` then splits the tokens into
per-source-line blocks so every line stays commentable.

Math uses KaTeX (`web/katex/`, loaded only when a document or comment has a
formula); emoji shortcodes use GitHub's table (`web/gemoji.min.js`). Both are
vendored through `copy-deps.js` and recorded in `ASSETS-PROVENANCE.txt`.

This page lists what GitHub renders and where Hunkyard stands. Status:
**yes** = rendered like GitHub, **differs** = rendered with a deliberate
difference, **no** = not supported yet.

## Document renderer (`documentMd` in `web/app.js`)

| Feature | GitHub syntax | Status | Where |
| --- | --- | --- | --- |
| Headings, emphasis, lists, links, images, code spans, block quotes, thematic breaks | CommonMark | yes | markdown-it |
| Hard line breaks | trailing `\` or two spaces | yes | markdown-it |
| Raw HTML | `<details>`, `<kbd>`, `<sub>`, `<picture>`, ... | yes | markdown-it (`html: true`) |
| Disallowed raw HTML | `<script>`, `<style>`, `<iframe>`, `<title>`, `<textarea>`, `<xmp>`, `<noembed>`, `<noframes>`, `<plaintext>` | yes: shown as text (GFM tagfilter) | `hunkyard-gfm.js` |
| Tables | pipes, `:--:` alignment, `\|` escapes | yes | markdown-it |
| Fenced code with syntax highlighting | ```` ```lang ```` | yes | Shiki (`hunkyard-code-highlight.js`) |
| Mermaid diagrams (flowchart, sequence, gantt, class, state, ER, pie, ...) | ```` ```mermaid ```` | yes | `hunkyard-renderer-mermaid.js` |
| Strikethrough | `~~text~~` and `~text~` | yes | `hunkyard-gfm.js` |
| Task lists | `- [ ]` / `- [x]` | yes: disabled checkboxes | `hunkyard-gfm.js` |
| Autolinks | `<https://...>`, `https://...`, `www.example.com`, `user@example.com` | yes | markdown-it linkify + `hunkyard-gfm.js` (`www.`) |
| Alerts | `> [!NOTE]`, `[!TIP]`, `[!IMPORTANT]`, `[!WARNING]`, `[!CAUTION]` | yes | `hunkyard-gfm.js` |
| Footnotes | `text[^1]` + `[^1]: note` | differs: the note renders where it is written, not collected at the end of the page, so its source lines stay commentable. Numbering and links match GitHub. | `hunkyard-gfm.js` |
| Heading anchors | `#hello-world`, duplicates `-1`, `-2` | yes: GitHub slug rules (punctuation dropped, spaces to `-`, no collapsing) | `app.js`, `slugifyHeading` |
| Straight quotes and dashes | `"quote"`, `--` | yes: no typographer, text renders as written | `app.js` |
| YAML front matter | `---` block at the top | yes: shown as a table on GitHub, as a highlighted YAML block here | `rewriteFrontmatterAsYamlFence` |
| Relative images | `![](img/a.png)` | yes | `rewriteImageSrcs` |
| Math | `$x$`, `` $`x`$ ``, `$$x$$`, `$$` blocks, ```` ```math ```` | yes: KaTeX, loaded on the first formula. A `$` followed by a letter or digit does not close math, so `$5 and $10` and `$HOME/$PATH` stay text. | `hunkyard-gfm.js`, `hunkyard-math.js` |
| Emoji shortcodes | `:tada:`, `:+1:` | yes: GitHub's own table (gemoji). Custom GitHub images such as `:octocat:` stay text. | `hunkyard-gfm.js`, `gemoji.min.js` |
| GeoJSON / TopoJSON maps, STL 3D models | ```` ```geojson ````, ```` ```topojson ````, ```` ```stl ```` | no: shown as highlighted code | - |
| Color chips | `` `#0969DA` `` | n/a: GitHub shows these in issues and PRs, not in files | - |
| @mentions, `#123` issue refs, commit SHAs | | n/a: GitHub links these in issues and PRs, not in files | - |

## Comment renderer (`commentMd` in `web/app.js`)

Review comments use `hunkyard-gfm.js` too, then the GitHub comment sanitizer
allowlist (`web/comment-html.js`), which keeps the task-list, alert and math
classes and turns any `<input>` into a disabled checkbox. Differences from
documents:

- Footnotes are off: ids from separate comments would collide on one page.
- Math renders after mounting. The sanitizer would strip KaTeX's markup, so
  formulas leave the renderer as `<code class="math-*">` source and
  `hunkyard-math.js` replaces them once the comment is on the page.
- Comments keep smart quotes (`typographer` without `replacements`).

Live mode (`live-mode.panel-render.js`) still renders comments with plain
markdown-it.

## Accessibility

| What | How |
| --- | --- |
| Math | KaTeX writes MathML next to the visual formula (`output: 'htmlAndMathml'`); the visual part is `aria-hidden`, so screen readers read the formula. |
| Task lists | Each item's text is the `<label>` of its disabled checkbox: announced as "reviewed, checkbox, checked". |
| Alerts | `role="note"` with a visible title; when an alert spans several line blocks, the later blocks carry the type as `aria-label`. The icon is decorative CSS. Title colours meet 4.5:1 on page and comment backgrounds in both themes. |
| Footnotes | DPUB roles (`doc-noteref`, `doc-footnote`, `doc-backlink`); references are named "Footnote N", back links "Back to reference N"; visible focus ring. |
| Tables | Header cells carry `scope="col"`. |
| Heading anchors | Named "Link to section: <heading>". |
| Mermaid | A diagram without `accTitle` is named by its kind ("Flowchart diagram"). Add `accTitle:` / `accDescr:` to a diagram for a fuller description. |
| Emoji | Plain Unicode characters, which screen readers announce by name. |

`test/e2e/tests/comment-markdown.spec.ts` checks these in a browser with axe.

## Adding a GitHub feature

1. Add the parser rule to `web/hunkyard-gfm.js` and tests to
   `web/__tests__/hunkyard-gfm.test.js`. Give the output an accessible name and
   role where it has meaning beyond its text.
2. If the new token spans several source lines, check how
   `buildLineBlocks` splits it (`web/hunkyard-line-blocks.js`); tokens without a
   `map` are skipped.
3. Style it in `web/style.css` under `.line-content`, with any colours as
   theme variables in all four `web/theme.css` blocks.
4. If comments should show it, allow its classes in `web/comment-html.js`.
5. Update the tables above.
