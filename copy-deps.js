import { cpSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync, unlinkSync } from "fs";
import { gemoji } from "gemoji";
import { execSync } from "child_process";
import { vendoredAssets } from "./scripts/vendored-assets.mjs";
import { buildPierre } from "./scripts/build-pierre.mjs";

const dest = "web";

// markdown-it
cpSync("node_modules/markdown-it/dist/browser/markdown-it.umd.min.js", `${dest}/markdown-it.min.js`);

// DOMPurify — used to sanitize HTML enabled in comment Markdown.
cpSync("node_modules/dompurify/dist/purify.min.js", `${dest}/dompurify.min.js`);

// mermaid
cpSync("node_modules/mermaid/dist/mermaid.min.js", `${dest}/mermaid.min.js`);

// KaTeX — math in Markdown ($…$, $$…$$, ```math). Loaded on first use by
// hunkyard-math.js. Every supported browser reads woff2, so only those fonts ship;
// the stylesheet drops the woff/ttf fallbacks and the fonts/ directory prefix.
const katexDir = `${dest}/katex`;
rmSync(katexDir, { recursive: true, force: true });
mkdirSync(katexDir);
cpSync("node_modules/katex/dist/katex.min.js", `${katexDir}/katex.min.js`);
for (const font of readdirSync("node_modules/katex/dist/fonts").filter(name => name.endsWith(".woff2"))) {
  cpSync(`node_modules/katex/dist/fonts/${font}`, `${katexDir}/${font}`);
}
writeFileSync(`${katexDir}/katex.min.css`, readFileSync("node_modules/katex/dist/katex.min.css", "utf8")
  .replace(/url\(fonts\/([^)]+\.woff2)\) format\("woff2"\)(?:,url\([^)]+\) format\("[^"]+"\))*/g, 'url($1) format("woff2")'));

// gemoji — GitHub's emoji shortcode table (:tada: → 🎉), as window.hunkyard.emojiShortcodes.
const shortcodes = {};
for (const entry of gemoji) for (const name of entry.names) shortcodes[name] = entry.emoji;
writeFileSync(`${dest}/gemoji.min.js`,
  "window.hunkyard=window.hunkyard||{};window.hunkyard.emojiShortcodes=" + JSON.stringify(shortcodes) + ";\n");

// @sanity/diff-match-patch — ESM-only, bundle to IIFE with esbuild
// Expose makeDiff, cleanupSemantic, and constants as window.DiffMatchPatch
const dmpEntry = `${dest}/_dmp-entry.js`;
writeFileSync(dmpEntry, `\
import {makeDiff, cleanupSemantic, DIFF_DELETE, DIFF_EQUAL, DIFF_INSERT} from '@sanity/diff-match-patch';
window.DiffMatchPatch = {makeDiff, cleanupSemantic, DIFF_DELETE, DIFF_EQUAL, DIFF_INSERT};
`);
execSync(`npx --no-install esbuild ${dmpEntry} --bundle --format=iife --minify --outfile=${dest}/diff-match-patch.min.js`, { stdio: 'inherit' });
// Clean up temporary entry file
unlinkSync(dmpEntry);

// @pierre/diffs + Shiki — split ESM bundle, gzipped, in web/pierre/.
await buildPierre();

// Keep the generator, manifest, and embedded minified files in a closed set.
// This prevents an ungenerated, self-attested *.min.js file from entering the
// binary through web/embed.go's broad *.js pattern.
const generatedAssets = vendoredAssets.map(asset => asset.path).sort();
const manifestedAssets = readFileSync("ASSETS-PROVENANCE.txt", "utf8")
  .split("\n")
  .filter(line => line && !line.startsWith("#"))
  .map(line => line.split(/\s+/)[1])
  .sort();
const embeddedAssets = readdirSync(dest)
  .filter(name => name.endsWith(".min.js"))
  .map(name => `${dest}/${name}`)
  .concat([`${dest}/pierre`, `${dest}/katex`])
  .sort();
for (const [label, assets] of [["manifest", manifestedAssets], ["web directory", embeddedAssets]]) {
  if (JSON.stringify(assets) !== JSON.stringify(generatedAssets)) {
    throw new Error(`${label} vendored assets do not match copy-deps.js outputs`);
  }
}

console.log("Frontend deps copied to web/");
