import { readdirSync } from "node:fs";
import path from "node:path";

import { defineConfig } from "@lingui/cli";

import base from "./lingui.config";

/**
 * The catalogues the app loads (vite.config.ts), a superset of the ones
 * `lingui extract` writes (lingui.config.ts): also the translation layers
 * that do not come from Sona's source, so extraction never creates or
 * rewrites them (docs/frontend-architecture.md, "Strings and locales").
 *
 * - `apple/{locale}`: Apple Music's own translations of the messages mapped
 *   to an Apple key (`cmd/i18ntool import`; gitignored).
 * - `community/{locale}`: translations of the other messages, made for
 *   Sona.
 *
 * Lingui compiles only the locales its config lists, so the layers' files
 * add theirs.
 */
const i18nDir = path.join(import.meta.dirname, "src/shared/i18n");

function localesIn(dir: string): string[] {
  try {
    return readdirSync(path.join(i18nDir, dir))
      .filter((f) => f.endsWith(".po"))
      .map((f) => f.slice(0, -3));
  } catch {
    return []; // no catalogues in this layer yet
  }
}

export default defineConfig({
  ...base,
  locales: [...new Set([...base.locales, ...localesIn("apple"), ...localesIn("community")])],
  catalogs: [
    ...(base.catalogs ?? []),
    { path: "<rootDir>/src/shared/i18n/apple/{locale}", include: [] },
    { path: "<rootDir>/src/shared/i18n/community/{locale}", include: [] },
  ],
});
