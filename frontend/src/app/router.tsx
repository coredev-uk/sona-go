import { createHashHistory, createRouter } from "@tanstack/react-router";

import { rootRoute } from "@/shared/router";

import { routes } from "./compose";
import { AppShell } from "./shell/AppShell";

rootRoute.update({ component: AppShell });

// The children's union type keeps every module's paths, params and search typed.
const routeTree = rootRoute.addChildren(routes);

/**
 * Hash history: Wails' asset server has no SPA fallback, so a path-based URL
 * would 404 on reload. Code-based routes keep the tree explicit.
 */
export const router = createRouter({
  routeTree,
  history: createHashHistory(),
  defaultPreload: "intent",
  scrollRestoration: true,
  scrollToTopSelectors: ["#content"],
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
