import path from "node:path";

import { lingui } from "@lingui/vite-plugin";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";
import { defineConfig } from "vitest/config";

export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  resolve: {
    alias: {
      "@bindings": path.resolve(
        import.meta.dirname,
        "bindings/github.com/coredev-uk/sona-go/internal",
      ),
      "@": path.resolve(import.meta.dirname, "src"),
    },
  },
  // React Compiler memoises components and hooks at build time, so parents'
  // inline props stay stable and memo'd rows and tiles skip re-rendering.
  // Lingui compiles its message macros natively (no Babel pass) and turns
  // `.po` catalogues into JS modules on import.
  // The Wails plugin needs generated bindings (`go tool wails3 generate
  // bindings -ts`); unit tests do not touch the runtime, so they skip it.
  plugins: [
    lingui({ macroTransform: true }),
    react(),
    babel({ presets: [reactCompilerPreset()] }),
    tailwindcss(),
    ...(process.env.VITEST ? [] : [wails("./bindings")]),
  ],
  build: {
    target: "es2022",
    chunkSizeWarningLimit: 800,
    // Two documents: the main window and the mini player window.
    rollupOptions: {
      input: {
        main: path.resolve(import.meta.dirname, "index.html"),
        miniplayer: path.resolve(import.meta.dirname, "miniplayer.html"),
      },
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    // Activates the English catalogue, as the app does before its first render.
    setupFiles: ["src/shared/i18n/test-setup.ts"],
  },
});
