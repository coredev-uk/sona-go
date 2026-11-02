import { readdirSync } from "node:fs";
import path from "node:path";

import { defineConfig } from "@lingui/cli";

import base from "./lingui.config";

/**
 * The catalogues the app loads (vite.config.ts), a superset of the ones
 * `lingui extract` writes (lingui.config.ts): also the community
 * translations (`community/{locale}`), made for Sona, which do not come
 * from Sona's source, so extraction never creates or rewrites them
 * (docs/frontend-architecture.md, "Strings and locales"). Apple Music's own
 * translations are not built in: Go fetches them at run time.
 *
 * Lingui compiles only the locales its config lists, so the community
 * catalogues add theirs.
 */
const communityDir = path.join(import.meta.dirname, "src/shared/i18n/community");

function communityLocales(): string[] {
  try {
    return readdirSync(communityDir)
      .filter((f) => f.endsWith(".po"))
      .map((f) => f.slice(0, -3));
  } catch {
    return []; // no community catalogues yet
  }
}

export default defineConfig({
  ...base,
  locales: [...new Set([...base.locales, ...communityLocales()])],
  catalogs: [
    ...(base.catalogs ?? []),
    { path: "<rootDir>/src/shared/i18n/community/{locale}", include: [] },
  ],
});
