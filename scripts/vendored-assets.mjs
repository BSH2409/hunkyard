import { createHash } from "node:crypto";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

// sha256 of a vendored asset. A generated directory (web/pierre) hashes as the
// sha256 of its sorted "name sha256" listing, so one provenance row covers
// every chunk and any added, removed or changed file changes the hash.
export function hashAsset(path) {
  if (!statSync(path).isDirectory()) {
    return createHash("sha256").update(readFileSync(path)).digest("hex");
  }
  const listing = readdirSync(path)
    .sort()
    .map(name => `${name} ${createHash("sha256").update(readFileSync(join(path, name))).digest("hex")}`)
    .join("\n");
  return createHash("sha256").update(listing).digest("hex");
}

export const vendoredAssets = [
  {
    path: "web/dompurify.min.js",
    sources: ["dompurify@3.5.0"],
    packagePath: "dist/purify.min.js",
    kind: "copy",
  },
  {
    path: "web/markdown-it.min.js",
    sources: ["markdown-it@15.0.3"],
    packagePath: "dist/browser/markdown-it.umd.min.js",
    kind: "copy",
  },
  {
    path: "web/snapdom.min.js",
    sources: ["@zumer/snapdom@3.7.6"],
    packagePath: "dist/snapdom.js",
    kind: "copy",
  },
  {
    path: "web/mermaid.min.js",
    sources: ["mermaid@12.3.0"],
    packagePath: "dist/mermaid.min.js",
    kind: "copy",
  },
  {
    path: "web/katex",
    sources: ["katex@0.19.1"],
    packagePath: "-",
    kind: "generated",
  },
  {
    path: "web/gemoji.min.js",
    sources: ["gemoji@8.1.0"],
    packagePath: "-",
    kind: "generated",
  },
  {
    path: "web/diff-match-patch.min.js",
    sources: ["@sanity/diff-match-patch@3.2.1", "esbuild@0.28.2"],
    packagePath: "-",
    kind: "generated",
  },
  {
    path: "web/pierre",
    sources: ["@pierre/diffs@1.6.4", "shiki@4.6.0", "@shikijs/themes@4.6.0", "esbuild@0.28.2"],
    packagePath: "-",
    kind: "generated",
  },
];
