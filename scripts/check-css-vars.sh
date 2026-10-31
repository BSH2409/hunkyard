#!/bin/bash
# Check that all var(--xxx) references resolve to defined CSS custom properties,
# that all definitions are actually referenced, and that theme.css vars appear
# in all 4 theme blocks.
set -e

# ── Allowlists ──────────────────────────────────────────────────────────────

# Variables set dynamically via JS or intentionally unreferenced.
# --diffs-* are read by @pierre/diffs inside its shadow root;
# --diffs-token-light/-dark are also set inline on Shiki token spans in
# fenced code (web/hunkyard-code-highlight.js).
# --hunkyard-find-* are read by the find highlights injected into Pierre
# (web/hunkyard-find.js).
DEAD_VAR_ALLOWLIST="
--hunkyard-find-bg --hunkyard-find-current-bg --hunkyard-find-current-fg
--font-sans --header-height --review-chrome-height --file-tree-width --comments-panel-width --story-rail-width --p
--hunkyard-border-strong --hunkyard-dur-base --hunkyard-dur-slow --hunkyard-ease-in --hunkyard-ease-out
--hunkyard-fg-muted --hunkyard-fg-secondary --hunkyard-header-height
--hunkyard-r-sm --hunkyard-r-xl
--hunkyard-live-marker-bg --hunkyard-live-marker-border --hunkyard-live-marker-fg --hunkyard-live-marker-shadow
--hunkyard-live-reanchor-active-outline
--diffs-font-family --diffs-font-size --diffs-line-height --diffs-tab-size --diffs-token-light --diffs-token-dark --diffs-modified-color-override
--hunkyard-diff-tint-mix
--diffs-bg-deletion-override --diffs-bg-addition-override
--diffs-bg-deletion-number-override --diffs-bg-addition-number-override
--diffs-bg-deletion-emphasis-override --diffs-bg-addition-emphasis-override --diffs-fg-number-override
--hunkyard-palette-bg --hunkyard-palette-fg --hunkyard-palette-accent --hunkyard-palette-on-accent
--hunkyard-palette-muted --hunkyard-palette-border --hunkyard-palette-surface --hunkyard-palette-elevated
--hunkyard-palette-red --hunkyard-palette-green --hunkyard-palette-yellow --hunkyard-palette-orange --hunkyard-palette-purple
--hunkyard-palette-shadow --hunkyard-palette-overlay --hunkyard-palette-blue --hunkyard-palette-cyan
--hunkyard-palette-author-0 --hunkyard-palette-author-1 --hunkyard-palette-author-2
--hunkyard-palette-author-3 --hunkyard-palette-author-4 --hunkyard-palette-author-5
"

# Variables that legitimately exist in only some theme blocks (not in all 4
# custom-property blocks)
BLOCK_ALLOWLIST="--hunkyard-dur-base --hunkyard-dur-fast --hunkyard-dur-slow --hunkyard-ease --hunkyard-ease-in --hunkyard-ease-out --hunkyard-focus --hunkyard-font-body --hunkyard-font-mono --hunkyard-r-lg --hunkyard-r-md --hunkyard-r-sm --hunkyard-r-xl"

# ── Extract refs and defs ───────────────────────────────────────────────────

# All var(--xxx) references (POSIX ERE, works on macOS and Linux)
REFS=$(grep -oE 'var\(--[a-zA-Z0-9_-]+' web/style.css web/style-live.css web/theme.css web/hunkyard-palette.css 2>/dev/null | sed 's/.*var(//' | sort -u)

# All --xxx: definitions (use perl for lookahead, portable)
DEFS=$(perl -nle 'print $1 if /^\s*(--[a-zA-Z0-9_-]+)\s*:/' web/theme.css web/style.css web/style-live.css web/hunkyard-palette.css 2>/dev/null | sort -u)

# Add dynamic vars to definitions for the undefined-ref check
DEFS_PLUS_DYNAMIC="$DEFS"
for v in $DEAD_VAR_ALLOWLIST; do
    DEFS_PLUS_DYNAMIC=$(printf '%s\n%s' "$DEFS_PLUS_DYNAMIC" "$v")
done
DEFS_PLUS_DYNAMIC=$(echo "$DEFS_PLUS_DYNAMIC" | sort -u)

# ── Check A: Undefined references ──────────────────────────────────────────

MISSING=$(comm -23 <(echo "$REFS") <(echo "$DEFS_PLUS_DYNAMIC"))

if [ -n "$MISSING" ]; then
    echo "ERROR: Undefined CSS variables (referenced but never defined):"
    echo "$MISSING"
    exit 1
fi

echo "OK: All CSS variable references resolve to definitions."

# ── Check B: Dead definitions ───────────────────────────────────────────────

# Build allowlist lookup
DEAD_ALLOW=""
for v in $DEAD_VAR_ALLOWLIST; do
    DEAD_ALLOW=$(printf '%s\n%s' "$DEAD_ALLOW" "$v")
done
DEAD_ALLOW=$(echo "$DEAD_ALLOW" | sort -u)

DEAD=$(comm -23 <(echo "$DEFS") <(echo "$REFS"))
# Remove allowlisted vars
if [ -n "$DEAD_ALLOW" ]; then
    DEAD=$(comm -23 <(echo "$DEAD") <(echo "$DEAD_ALLOW"))
fi

if [ -n "$DEAD" ]; then
    echo "ERROR: Dead CSS variables (defined but never referenced via var()):"
    echo "$DEAD"
    exit 1
fi

echo "OK: No dead CSS variable definitions."

# ── Check C: 4-block completeness (theme.css only) ─────────────────────────

# Extract variables defined in theme.css (not style.css) and only from the
# 4 theme custom-property blocks.
# Strategy: parse theme.css, track which block we're in, collect var names.

THEME_FILE="web/theme.css"

# We use perl to parse the file and emit "BLOCK_NAME\tVAR_NAME" pairs.
# The 4 blocks are identified by their opening patterns:
#   :root {                                -> root
#   prefers-color-scheme: light ... {      -> system-light (inside @media)
#   [data-theme="dark"] {                  -> dark
#   [data-theme="light"] {                 -> light

BLOCK_VARS=$(perl -e '
    use strict;
    use warnings;
    my $block = "";
    my $depth = 0;

    while (<>) {
        # Detect block openings (only at depth 0 or 1 for the @media case)
        if (/^\s*:root\s*\{/ && $depth == 0) {
            $block = "root";
            $depth = 1;
            next;
        }
        if (/prefers-color-scheme:\s*light/ && $depth == 0) {
            # @media block — we will match the inner html:not block
            $depth = 1;
            $block = "";
            next;
        }
        if (/html:not\(\[data-theme\]\)\s*\{/ && $depth == 1 && $block eq "") {
            $block = "system-light";
            $depth = 2;
            next;
        }
        if (/\[data-theme="dark"\]\s*\{/ && $depth == 0) {
            $block = "dark";
            $depth = 1;
            next;
        }
        if (/\[data-theme="light"\]\s*\{/ && $depth == 0) {
            $block = "light";
            $depth = 1;
            next;
        }

        # Track braces for blocks we do not care about
        if ($block eq "") {
            my $opens = () = /\{/g;
            my $closes = () = /\}/g;
            $depth += $opens - $closes;
            $depth = 0 if $depth < 0;
            next;
        }

        # Collect variable definitions
        if (/^\s*(--[a-zA-Z0-9_-]+)\s*:/) {
            print "$block\t$1\n";
        }

        # Track closing braces
        my $opens = () = /\{/g;
        my $closes = () = /\}/g;
        $depth += $opens - $closes;
        if ($depth <= 0) {
            $block = "";
            $depth = 0;
        }
    }
' "$THEME_FILE")

# Collect the union of all vars across the 4 blocks
ALL_THEME_VARS=$(echo "$BLOCK_VARS" | cut -f2 | sort -u)

# Build allowlist lookup for block check
BLOCK_ALLOW=""
for v in $BLOCK_ALLOWLIST; do
    BLOCK_ALLOW=$(printf '%s\n%s' "$BLOCK_ALLOW" "$v")
done
BLOCK_ALLOW=$(echo "$BLOCK_ALLOW" | sort -u)

ERRORS=""
for var in $ALL_THEME_VARS; do
    # Skip allowlisted vars
    if [ -n "$BLOCK_ALLOW" ] && echo "$BLOCK_ALLOW" | grep -qxF -- "$var"; then
        continue
    fi

    MISSING_BLOCKS=""
    for block in root system-light dark light; do
        if ! printf '%s\n' "$BLOCK_VARS" | grep -qxF -- "${block}	${var}"; then
            MISSING_BLOCKS="$MISSING_BLOCKS $block"
        fi
    done

    if [ -n "$MISSING_BLOCKS" ]; then
        ERRORS=$(printf '%s\n  %s missing from:%s' "$ERRORS" "$var" "$MISSING_BLOCKS")
    fi
done

if [ -n "$ERRORS" ]; then
    echo "ERROR: Theme variables not defined in all 4 blocks:$ERRORS"
    exit 1
fi

echo "OK: All theme variables defined in all 4 blocks."

# Colours in component rules must come from theme variables so every code
# theme recolours them (hunkyard-palette.css maps the variables to the palette).
# Allowed: custom-property definitions, comments, neutral black/white
# shadows, and the white page behind the reviewed app's iframe.
LITERALS=$(perl -ne '
  if ($in) { next unless s{^.*?\*/}{}; $in = 0 }   # inside a /* */ block
  next if /iframe-frame-literal-ok/;
  s{/\*.*?\*/}{}g;                                 # inline comments
  if (s{/\*.*$}{}) { $in = 1 }                      # comment opens here
  next if /^\s*--/;                                 # custom property definitions
  s/rgba?\(\s*(0|255)\s*,\s*\1\s*,\s*\1\s*(,[^)]*)?\)//g;   # neutral black/white
  print "$ARGV:$.: $_" if /#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/;
  close ARGV if eof;' web/style.css web/style-live.css)
if [ -n "$LITERALS" ]; then
  echo "ERROR: Hardcoded colours in component CSS (use a --hunkyard-* variable):"
  echo "$LITERALS"
  exit 1
fi
echo "OK: No hardcoded colours in component CSS."
