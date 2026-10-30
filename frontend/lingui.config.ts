import { defineConfig } from "@lingui/cli";
import { formatter } from "@lingui/format-po";

/**
 * Message catalogues (docs/frontend-architecture.md, "Strings and
 * locales"). English is the source locale: message IDs are generated from
 * the English text, and `en.po` is bundled. `en-XA` is the pseudo-locale:
 * the Vite plugin accents and stretches every English message, so a string
 * that was not extracted stands out as plain English.
 */
export default defineConfig({
  sourceLocale: "en",
  locales: ["en", "en-XA"],
  pseudoLocale: { locale: "en-XA", prepend: "⟦", append: "⟧", extend: 0.35 },
  fallbackLocales: { default: "en" },
  catalogs: [
    {
      path: "<rootDir>/src/shared/i18n/locales/{locale}",
      include: ["<rootDir>/src"],
      exclude: ["**/*.test.ts", "**/*.test.tsx", "**/node_modules/**"],
    },
  ],
  format: formatter({ lineNumbers: false }),
});
