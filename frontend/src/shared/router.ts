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
