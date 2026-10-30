import js from "@eslint/js";
import lingui from "eslint-plugin-lingui";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

/**
 * Layer boundaries (docs/frontend-architecture.md): features use shared,
 * bindings, packages, and other features only through their index
 * (`@/features/<name>`); shared uses neither features nor the app.
 */
const featureBoundaries = [
  {
    regex: "^@/features/[^/]+/",
    message:
      "Import another feature through its index: @/features/<name>. Inside a feature, use relative paths.",
  },
  {
    regex: "^@/app(/|$)",
    message:
      "Features must not import the app layer; contribute through the module object instead.",
  },
  {
    regex: "^(\\.\\./)+(app|features|shared)(/|$)",
    message: "Use @/shared/… or @/features/<name> instead of climbing out of the feature.",
  },
];

const restrict = (patterns) => ({ "no-restricted-imports": ["error", { patterns }] });

/**
 * Every user-visible string goes through Lingui (docs/frontend-architecture.md,
 * "Strings and locales"). Strings the user never reads are recognised by
 * where they sit (class names, keys, union-typed props via TS types, calls
 * such as `cn`) or by shape: no whitespace and not capitalised (IDs, routes,
 * CSS tokens, event names).
 */
const unlocalizedStrings = {
  useTsTypes: true,
  ignore: ["^(?![A-Z])\\S+$", "^use client$"],
  ignoreNames: [
    { regex: { pattern: "className$", flags: "i" } },
    { regex: { pattern: "^[A-Z0-9_]+$" } },
    { regex: { pattern: "^(data|aria)-(?!label|description|valuetext|roledescription|placeholder)" } },
    "id",
    "key",
    "keys",
    "to",
    "href",
    "src",
    "type",
    "role",
    "style",
    "transform",
    "accept",
    "autoComplete",
    "inputMode",
    "queryKey",
    "mutationKey",
  ],
  ignoreFunctions: [
    "cn",
    "cva",
    "clsx",
    "console.*",
    "Error",
    "*.addEventListener",
    "*.removeEventListener",
    "*.querySelector",
    "*.querySelectorAll",
    "*.closest",
    "*.matches",
    "*.matchMedia",
    "useMediaQuery",
    "Events.On",
    "Events.Emit",
  ],
};

export default tseslint.config(
  { ignores: ["dist", "bindings"] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": ["warn", { allowConstantExport: true }],
    },
  },
  {
    // ShadCN components export variants next to components by design.
    files: ["src/shared/ui/**/*.tsx"],
    rules: { "react-refresh/only-export-components": "off" },
  },
  {
    files: ["src/shared/**/*.{ts,tsx}"],
    rules: restrict([
      {
        regex: "^(@/|(\\.\\./)+)(features|app)(/|$)",
        message: "Shared code must not depend on features or the app.",
      },
    ]),
  },
  {
    // Nested folders inside a feature may climb within it.
    files: ["src/features/*/*/**/*.{ts,tsx}"],
    rules: restrict(featureBoundaries),
  },
  {
    // A feature's top-level files: "../" would already reach a sibling feature.
    files: ["src/features/*/*.{ts,tsx}"],
    rules: restrict([
      ...featureBoundaries,
      {
        regex: "^\\.\\./",
        message: "Import another feature through its index: @/features/<name>.",
      },
    ]),
  },
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/**/*.test.{ts,tsx}", "src/shared/testing/**", "src/shared/i18n/test-setup.ts"],
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
    },
    plugins: { lingui },
    rules: {
      ...lingui.configs["flat/recommended"].rules,
      "lingui/no-unlocalized-strings": ["error", unlocalizedStrings],
    },
  },
);
