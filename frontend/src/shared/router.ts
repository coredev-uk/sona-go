import { createRootRoute } from "@tanstack/react-router";

/**
 * The route every feature page hangs off. It lives here so features can
 * name it as their parent; the app gives it the shell component
 * (app/router.tsx) before the router is created.
 */
export const rootRoute = createRootRoute();

/**
 * Options for a top-level page route, spread into `createRoute`. Routes must
 * be constants: created inline inside addChildren() their types widen to
 * `any` and every typed link/search/param check is lost.
 */
export const childRoute = <P extends string>(path: P) =>
  ({ getParentRoute: () => rootRoute, path }) as const;

declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    /**
     * The page takes the whole window: the shell draws only the title
     * bar's band (the window's buttons and the mark, over the page's top
     * `--title-bar-height` pixels) and no sidebar, top bar, asides or
     * bottom bars. The first-run setup is one.
     */
    fullWindow?: boolean;
  }
}
