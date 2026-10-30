import type { QueryClient, QueryKey } from "@tanstack/react-query";
import type { AnyRoute, UseNavigateResult } from "@tanstack/react-router";
import type { Events } from "@wailsio/runtime";
import type { LucideIcon } from "lucide-react";
import type { ComponentType, ReactNode } from "react";

import type { AnyActionDef } from "./actions/registry";

/**
 * A page route: a constant made with `createRoute({ ...childRoute(path), … })`
 * (shared/router.ts), so its path, params and search stay typed.
 */
export type RouteDef = AnyRoute;

/** Where a sidebar link sits: the top list, under "Library", or at the foot. */
export type NavSection = "primary" | "library" | "footer";

export interface NavItem<TPath extends string = string> {
  section: NavSection;
  label: string;
  icon: LucideIcon;
  /** One of the module's own route paths. */
  to: TPath;
  /** Position inside the section, ascending. */
  order: number;
  /** Active only on this exact path (Listen Now at "/"). */
  exact?: boolean;
}

/** Go events with typed payloads (bindings/…/eventdata.d.ts). */
export type GoEventName = keyof Events.CustomEvents;
export type GoEventData<E extends GoEventName> = Events.CustomEvents[E];

/** What event handlers and `init` get from the shell. */
export interface EventContext {
  qc: QueryClient;
  /** Marks every query under these key prefixes stale. */
  invalidate: (keys: readonly QueryKey[]) => void;
  navigate: UseNavigateResult<string>;
}

export type GoEventHandler<E extends GoEventName> = (
  data: GoEventData<E>,
  ctx: EventContext,
) => void;
export type GoEventHandlers = { [E in GoEventName]?: GoEventHandler<E> };

export interface ShortcutContext {
  /** The focused element the key went to, if any. */
  target: HTMLElement | null;
  /** Focus is in a text field (input, textarea, select, contenteditable). */
  typing: boolean;
}

/**
 * A window-wide key. The shell skips keys already handled, with Alt or
 * Meta, or inside an open menu, dialog or listbox; the first shortcut that
 * matches gets the key, and its default is prevented.
 */
export interface Shortcut {
  id: string;
  /** What it does, for docs and help: "Play or pause". */
  label: string;
  matches: (e: KeyboardEvent, ctx: ShortcutContext) => boolean;
  run: (e: KeyboardEvent) => void;
}

/** Places in the shell a module can render into. */
export interface ShellSlots {
  /** Wraps the shell (a context shared components read). */
  Provider?: ComponentType<{ children: ReactNode }>;
  /** Beside the page, right of it (the queue); renders null when closed. */
  Aside?: ComponentType;
  /** Covers the page column; the page is inert while `useOpen` is true. */
  overlay?: { Component: ComponentType; useOpen: () => boolean };
  /** Across the bottom of the window (the now-playing bar). */
  Bottom?: ComponentType;
  /** At the foot of the sidebar, above its footer links (the sync indicator). */
  SidebarFooter?: ComponentType;
  /**
   * Below the library links, in the sidebar's scrolling middle (pins, the
   * playlist tree). Sections stack in module order.
   */
  SidebarSection?: ComponentType;
  /** The top bar's search field, between the history buttons and the account (the first one wins). */
  TopBarSearch?: ComponentType;
}

/**
 * A feature, as registered in app/modules.ts. Annotate the exported module
 * with its route union, `FeatureModule<(typeof routes)[number]>`: an
 * annotation (not inference) keeps the router's types from depending on
 * themselves through Register.
 */
export interface FeatureModule<TRoute extends RouteDef = RouteDef> {
  id: string;
  routes: readonly TRoute[];
  nav?: readonly NavItem<TRoute["fullPath"]>[];
  shortcuts?: readonly Shortcut[];
  eventHandlers?: GoEventHandlers;
  /** Runs once when the shell connects to Go, after handlers are bound: initial snapshots. */
  init?: (ctx: EventContext) => void;
  actions?: readonly AnyActionDef[];
  shellSlots?: ShellSlots;
}
