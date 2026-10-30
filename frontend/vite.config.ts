import path from "node:path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
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
  // The Wails plugin needs generated bindings (`go tool wails3 generate
  // bindings -ts`); unit tests do not touch the runtime, so they skip it.
  plugins: [react(), tailwindcss(), ...(process.env.VITEST ? [] : [wails("./bindings")])],
  build: {
    target: "es2022",
    chunkSizeWarningLimit: 800,
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
