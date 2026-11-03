import type { QueryClient, QueryKey } from "@tanstack/react-query";
import type { AnyRoute, RouterHistory, UseNavigateResult } from "@tanstack/react-router";
import type { Events } from "@wailsio/runtime";
import type { LucideIcon } from "lucide-react";
import type { ComponentType, ReactNode } from "react";

import type { ItemActionDef } from "./actions/registry";

/**
 * A page route: a constant made with `createRoute({ ...childRoute(path), … })`
 * (shared/router.ts), so its path, params and search stay typed.
 */
export type RouteDef = AnyRoute;

/** Where a sidebar link sits: the top list, under "Library", or at the foot. */
export type NavSection = "primary" | "library" | "footer";

export interface NavItem<TPath extends string = string> {
  section: NavSection;
  /** Translated where shown (docs/frontend-architecture.md, "Strings and locales"): () => t`Albums`. */
  label: () => string;
  icon: LucideIcon;
  /** One of the module's own route paths. */
  to: TPath;
  /** Position inside the section, ascending. */
  order: number;
  /** Active only on this exact path (Listen Now at "/"). */
  exact?: boolean;
  /**
   * Default chords of its "go to" shortcut. Without them, primary links get
   * Ctrl+1…9 and library links Alt+1…9 by position.
   */
  keys?: readonly string[];
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

/** How shortcuts are listed in help, the palette and Settings, in this order. */
export const shortcutGroups = ["playback", "view", "pages", "navigation", "general"] as const;
export type ShortcutGroup = (typeof shortcutGroups)[number];

/**
 * Where a shortcut's keys work. "page" (the default): anywhere except in a
 * text field. "global": in text fields too, for chords with Ctrl, Alt or
 * Meta. Any other name is a context (a panel, "now-playing") that should
 * also say `when` it is active; keys in different contexts may repeat.
 */
export type ShortcutScope = "page" | "global" | (string & {});

/** What a shortcut gets when it runs (from a key or the command palette). */
export interface ShortcutContext {
  navigate: UseNavigateResult<string>;
  history: RouterHistory;
}

/**
 * A command with a stable id and default keys, declared in a module's
 * `shortcuts`. The user can rebind it (Settings → Shortcuts, stored by id),
 * the `?` sheet lists it and the Ctrl+K palette runs it. Adding one is one
 * entry:
 *
 *   { id: "miniplayer.toggle", label: () => t`Show or hide the mini player`,
 *     group: "view", keys: ["Ctrl+Shift+M"], run: () => toggleMini() }
 *
 * The shell's one keydown listener skips keys already handled and keys
 * inside an open menu, dialog or listbox; plain keys (no Ctrl, Alt or
 * Meta) never fire in text fields, and Space, Enter and arrows not on a
 * control that uses them. The first shortcut bound to the chord whose
 * scope allows it and whose `when` holds gets the key (default prevented).
 */
export interface Shortcut {
  /** Stable id, "<module>.<action>": settings store bindings under it. */
  id: string;
  /** What it does, in sentence case, translated where shown: () => t`Play or pause`. */
  label: () => string;
  group: ShortcutGroup;
  /** Default chords ("Space", "Ctrl+K", "Shift+ArrowRight"); [] for none. */
  keys: readonly string[];
  scope?: ShortcutScope;
  /** Extra words the palette matches, translated: () => t`resume stop`. */
  keywords?: () => string;
  icon?: LucideIcon;
  /** Available now; when false the key goes on to the next match and the palette hides it. */
  when?: () => boolean;
  /**
   * Why it doesn't apply to what is happening now (Next on a live station),
   * or undefined when it does. The key still takes the chord: it does
   * nothing and says why (a notification); the palette leaves it out.
   */
  unavailable?: () => string | undefined;
  /** Runs again while the key is held (volume, seeking). */
  repeat?: boolean;
  /** Left out of the command palette (Escape-style keys). */
  palette?: false;
  run: (ctx: ShortcutContext) => void;
}

/** Places in the shell a module can render into. */
export interface ShellSlots {
  /** Wraps the shell (a context shared components read). */
  Provider?: ComponentType<{ children: ReactNode }>;
  /** Beside the page, right of it (the queue); renders null when closed. */
  Aside?: ComponentType;
  /** Covers the page column; the page is inert and hidden (not painted) while `useOpen` is true. */
  overlay?: { Component: ComponentType; useOpen: () => boolean };
  /** Across the bottom of the window (the now-playing bar). */
  Bottom?: ComponentType;
  /**
   * Over the whole window, after everything else: a modal of the module's
   * own that covers the sidebar, top bar and player bar too (the first-run
   * setup). It portals itself; the window behind is inert while it is open.
   */
  Layer?: ComponentType;
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
  actions?: readonly ItemActionDef[];
  shellSlots?: ShellSlots;
}
