#!/usr/bin/env node
// Fails when two tracked paths, or two JS/TS module specifiers, differ only by
// letter case. They are distinct on Linux but collide on case-insensitive
// filesystems (macOS, Windows): one file shadows the other, or an import such
// as `./Appearance` resolves to `appearance.ts` instead of `Appearance.tsx`.
//
// Usage: node frontend/scripts/check-path-case.mjs (from anywhere in the repo).

import { execFileSync } from "node:child_process";

const git = (...args) => execFileSync("git", args, { encoding: "utf8" });

const root = git("rev-parse", "--show-toplevel").trim();
const files = git("-C", root, "ls-files", "-z").split("\0").filter(Boolean);

// Extensions a bundler or tsc resolves for an extensionless specifier.
const moduleExt = /\.(?:[cm]?[jt]sx?)$/;

/** Folded key -> (original key -> a tracked file that produces it). */
const keys = new Map();

function add(kind, original, file) {
  const folded = `${kind}:${original.normalize("NFC").toLowerCase()}`;
  let originals = keys.get(folded);
  if (!originals) {
    originals = new Map();
    keys.set(folded, originals);
  }
  if (!originals.has(original)) originals.set(original, file);
}

for (const file of files) {
  // The file and every directory above it.
  const parts = file.split("/");
  for (let i = 1; i <= parts.length; i++) {
    add("path", parts.slice(0, i).join("/"), file);
  }

  // The specifier that resolves to it: `a/B.tsx` is `a/B`; `a/b/index.ts` is
  // also `a/b`.
  if (!moduleExt.test(file)) continue;
  const specifier = file.replace(moduleExt, "");
  add("module", specifier, file);
  if (parts.length > 1 && parts.at(-1).replace(moduleExt, "") === "index") {
    add("module", parts.slice(0, -1).join("/"), file);
  }
}

let collisions = 0;
for (const [folded, originals] of keys) {
  if (originals.size < 2) continue;
  collisions++;
  const kind = folded.slice(0, folded.indexOf(":"));
  const list = [...originals]
    .map(([original, file]) =>
      kind === "path" || original === file ? original : `${original} (${file})`,
    )
    .join("  <->  ");
  console.error(`case collision (${kind}): ${list}`);
}

if (collisions > 0) {
  console.error(
    `\n${collisions} name(s) differ only by case; rename one side so the repo works on case-insensitive filesystems.`,
  );
  process.exit(1);
}
console.log(`No case collisions among ${files.length} tracked files.`);
