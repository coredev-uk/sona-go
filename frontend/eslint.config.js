import js from "@eslint/js";
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
);
