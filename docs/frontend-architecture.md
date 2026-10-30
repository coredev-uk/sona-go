# Frontend architecture

The frontend (`frontend/src`) is a shell plus feature modules. A feature is a
folder under `src/features/` whose `index.ts` exports one module object; the
app composes the router, the sidebar, Go event handling, shortcuts, shell
slots and item actions from the ordered list in `src/app/modules.ts`. Adding
an Apple Music page means adding a folder and one line in that list.

Visual direction is `docs/design.md`. beta.music.apple.com is a reference for
*functionality* only (pages, data, actions); every page is designed under
Sona's own direction. Before designing or restyling a page, read
`skill://impeccable` and `skill://design-taste-frontend` and follow them.

## Layout

```
src/
  app/                    the shell; may import anything
    main.tsx              entry: appearance binding, action registration, render
    modules.ts            the ordered list of registered feature modules
    compose.ts            routes, nav, shortcuts, slots, actions derived from modules
    router.tsx            root route + module routes → router, typed Register
    providers.tsx         QueryClient, Tooltip and Toaster providers
    bridge.ts             Go events → module handlers, module init, app:open, FirstPaint
    open.ts (+ test)      app:open link → route plan (pure)
    shortcuts.ts          the single window keydown dispatcher
    shell/                AppShell (slots), Sidebar (nav + footer slots), TopBar, StatusBanner
  shared/                 generic building blocks; never imports features or app
    module.ts             the module contract (types below)
    router.ts             rootRoute and childRoute(path)
    actions/              item action registry, ActionMenu (+ test)
    components/           Artwork, Backdrop, Badges (ExplicitMark, audio labels), CollectionHeader,
                          ConfirmDialog (ConfirmHost), CoverGrid, DetailStates, EmptyState,
                          ErrorBlock, LibraryButton, PageHeader, PlayButtons, RatingButtons
    tracks/               TrackList, Equaliser
    items/                Item (any-kind card DTO) rendering: itemLink, play registry
                          (registerItemPlayer, playItem), ItemMenu, ItemShelf, ItemGrid,
                          WideRow (editorial banners), TrackShelf, LinkList,
                          EditorialSections (renders Section kinds; Home, Discover, Radio)
    lib/                  art, errors (callFailure), format, keys, queries (isLibraryId), run,
                          item-state (useItemState: batched library state + rating;
                          changeItemState, addItemsToLibrary), library-actions (rateItem,
                          addToLibrary, removeFromLibrary), confirm, content-policy
                          (restricted accounts), traits, now-playing, useMediaQuery,
                          useContentVirtualizer, utils (cn)
    brand/                SonaMark
    styles/index.css      Tailwind entry and design tokens
    testing/              helpers for tests (query-cache invalidation probe)
    ui/                   ShadCN components
  features/<name>/        one folder per feature; public API is index.ts
    home                  For you (Apple's personal shelves, local fallback shelves)
    discover              Discover (New), rooms, multirooms, groupings, curators, Charts;
                          registers the song/album/playlist item players
    radio                 Radio (live stations, your stations, Apple's radio page), station
                          pages (/station/$id: schedule, unavailable state); playStation,
                          useOnAirProgramme; Create station on songs/albums/artists; the
                          station item player
    search                the top bar's search field (suggestions, recent searches), Search page
                          (Apple Music results by group, library scope, browse categories),
                          "see all" pages per group (/search/$group), profiles (/profile/$id)
    library               Recently Added, Artists, Albums, Songs, Music Videos, Made for You,
                          Favourite Songs, Playlists; useLibraryList, sync store, SyncIndicator,
                          sync invalidations; item actions (library, Favourite, Suggest Less,
                          Copy link); shell Provider (ConfirmHost, content policy)
    album, artist, playlist  detail pages + their queries; "Go to album/artist" actions
    player                now-playing bar, queue, now-playing view, playback mirror,
                          lyrics, shortcuts, playSongs/shuffleSongs/playNext/addToQueue;
                          AutoPlay toggle and station/AutoPlay section in the queue,
                          live radio line in place of the seek bar
    settings              Settings page and Playback/Appearance/Diagnostics sections,
                          appearance store, useSettings/useUpdateSettings
    auth                  Account section, sign-in reducer (+ test), useAuth/hasAccount,
                          auth invalidations (+ test)
```

Tests sit next to their code (`*.test.ts`).

## The module contract

`src/shared/module.ts`:

```ts
export type RouteDef = AnyRoute;

export type NavSection = "primary" | "library" | "footer";

export interface NavItem<TPath extends string = string> {
  section: NavSection;
  label: string;
  icon: LucideIcon;
  /** One of the module's own route paths. */
  to: TPath;
  /** Position inside the section, ascending. */
  order: number;
  /** Active only on this exact path (For you at "/"). */
  exact?: boolean;
}

export type GoEventName = keyof Events.CustomEvents;
export type GoEventData<E extends GoEventName> = Events.CustomEvents[E];

export interface EventContext {
  qc: QueryClient;
  /** Marks every query under these key prefixes stale. */
  invalidate: (keys: readonly QueryKey[]) => void;
  navigate: UseNavigateResult<string>;
}

export type GoEventHandler<E extends GoEventName> = (data: GoEventData<E>, ctx: EventContext) => void;
export type GoEventHandlers = { [E in GoEventName]?: GoEventHandler<E> };

export interface ShortcutContext {
  target: HTMLElement | null;
  /** Focus is in a text field (input, textarea, select, contenteditable). */
  typing: boolean;
}

export interface Shortcut {
  id: string;
  label: string;
  matches: (e: KeyboardEvent, ctx: ShortcutContext) => boolean;
  run: (e: KeyboardEvent) => void;
}

export interface ShellSlots {
  /** Wraps the shell (a context shared components read). */
  Provider?: ComponentType<{ children: ReactNode }>;
  /** Beside the page, right of it (the queue); renders null when closed. */
  Aside?: ComponentType;
  /** Covers the page column; the page is inert while `useOpen` is true. */
  overlay?: { Component: ComponentType; useOpen: () => boolean };
  /** Across the bottom of the window (the now-playing bar). */
  Bottom?: ComponentType;
  /** Below the library links, in the sidebar's scrolling middle (pins, playlist tree). */
  SidebarSection?: ComponentType;
  /** At the foot of the sidebar, above its footer links (the sync indicator). */
  SidebarFooter?: ComponentType;
  /** The top bar's search field, between the history buttons and the account (the first one wins). */
  TopBarSearch?: ComponentType;
}

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
```

Two typing rules keep TanStack Router fully typed:

1. **Routes are constants**, made with `createRoute({ ...childRoute("/path"), … })`
   from `@/shared/router`. `childRoute` sets `getParentRoute: () => rootRoute`.
   Created inline, a route's type widens and typed links/params/search break.
2. **Annotate the module** with its route union —
   `FeatureModule<typeof route>` or `FeatureModule<(typeof routes)[number]>`
   (`FeatureModule<never>` for a module without pages). The router's
   `Register` type is derived from the modules; inferring a module's type from
   its initializer (whose actions and handlers call the typed `navigate`)
   would make that type depend on itself.

A nav item can only link to one of its own module's routes (`to` is typed
as `TRoute["fullPath"]`).

### How the app composes modules

- `app/compose.ts`: `routes` is every module's routes in list order (typed as
  the union `ModuleRoute`); `nav` groups nav items by section and sorts by
  `order`; `shortcuts` and `slots` follow list order;
  `registerModuleActions()` puts every module's `actions` in the registry
  (called once in `main.tsx`, before the first render).
- `app/router.tsx`: `rootRoute.update({ component: AppShell })`, then
  `rootRoute.addChildren(routes)`; `Register` is declared from that router,
  so `Link`, `getRouteApi` and `navigate` check paths, params and search
  everywhere.
- `app/shell/Sidebar.tsx`: renders `nav.primary`, then a "Library" heading
  with `nav.library` (when non-empty), then the `SidebarFooter` slots and
  `nav.footer`.
- `app/shell/AppShell.tsx`: wraps the layout in every `Provider` slot, puts
  `Aside` slots beside `main#content`, `overlay` components over the page
  column (the page is `inert` while any overlay's `useOpen()` is true), and
  `Bottom` slots across the window's foot.

## Adding a feature (worked example)

A "Radio" page with a sidebar link and a Go event:

```ts
// src/features/radio/index.ts
import { createRoute } from "@tanstack/react-router";
import { Radio } from "lucide-react";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { RadioPage } from "./RadioPage";

const radioRoute = createRoute({ ...childRoute("/radio"), component: RadioPage });

export const radioModule: FeatureModule<typeof radioRoute> = {
  id: "radio",
  routes: [radioRoute],
  nav: [{ section: "primary", label: "Radio", icon: Radio, to: "/radio", order: 30 }],
  // Hypothetical event: any key of the generated Events.CustomEvents works.
  // eventHandlers: { "radio:changed": (data, { invalidate }) => invalidate([["catalog", "radio"]]) },
};
```

```ts
// src/app/modules.ts — the one line
import { radioModule } from "@/features/radio";
export const modules = [homeModule, searchModule, radioModule, /* … */] as const;
```

Inside `RadioPage.tsx`, use `getRouteApi("/radio")` for typed search/params,
shared components from `@/shared/…`, and other features only through their
index (`import { playSongs } from "@/features/player"`). Search-param
schemas live in the feature (`search-params.ts`) and are passed to
`validateSearch`. Nothing outside the feature folder and `modules.ts` changes.

## Item action registry

`src/shared/actions/registry.ts` — row/item menus list registered actions.

```ts
export interface ActionTargets { song: Song; album: Album; playlist: Playlist; artist: Artist }
export type ActionTargetKind = keyof ActionTargets;

/** Menu sections in display order; a separator divides neighbouring non-empty sections. */
export const actionGroups = ["queue", "item", "rating"] as const;
export type ActionGroup = (typeof actionGroups)[number];

/** Data read only when a menu opens (a song's rating); shared by `id`. */
export interface MenuData<K extends ActionTargetKind, V> {
  id: string;
  load(target: ActionTargets[K]): Promise<V>; // should not reject
}

// The collection a row is listed in; <TrackList container={{kind, id, editable}}>
// passes it (with the row's index) to each row's <ActionMenu container>.
export interface ActionContainer { kind: string; id: string; index: number; editable: boolean }

export interface ActionContext<K extends ActionTargetKind> {
  navigate: UseNavigateResult<string>;
  data<V>(data: MenuData<K, V>): V | undefined; // undefined until loaded
  container?: ActionContainer;    // e.g. "Remove from this playlist"
}

type Read<K, R> = (target: ActionTargets[K], ctx: ActionContext<K>) => R;

export interface ActionDef<K extends ActionTargetKind> {
  id: string;                     // unique, e.g. "player.play-next"
  target: K;
  label: string | Read<K, string>;
  icon: LucideIcon;
  filled?: Read<K, boolean>;      // filled icon (a favourite's star)
  group: ActionGroup;
  order: number;                  // within the group
  isAvailable?: Read<K, boolean>; // listed at all (default true)
  isEnabled?: Read<K, boolean>;   // greyed out when false (default true)
  data?: readonly MenuData<K, unknown>[];
  run: Read<K, void>;
}

export type AnyActionDef = { [K in ActionTargetKind]: ActionDef<K> }[ActionTargetKind];

export function registerActions(defs: readonly AnyActionDef[]): void; // throws on a duplicate id
export function actionsFor<K extends ActionTargetKind>(kind: K): readonly ActionDef<K>[];
export function menuSections<A extends { group: ActionGroup }>(actions: readonly A[]):
  { group: ActionGroup; actions: A[] }[];
```

Features declare actions in their module's `actions`; the app registers them.
`<ActionMenu kind="song" target={song} name={song.name} />`
(`shared/actions/ActionMenu.tsx`) renders the "more" menu for any target:
available actions by group and order, separators between non-empty groups,
menu data loaded on open.

Registered song actions today (album, artist and playlist menus get the
library and rating rows too; stations get Copy link):

| Group  | Order | Action | Module |
| ------ | ----- | ------ | ------ |
| queue  | 10 | Play next (disabled if not playable) | player |
| queue  | 20 | Add to queue (disabled if not playable) | player |
| item   | 10 | Go to album (if the song has an album) | album |
| item   | 20 | Go to artist (if the song has an artist) | artist |
| item   | 30 | Add to library (catalogue items not in the library) | library |
| item   | 35 | Remove from library (with confirmation; not playlists) | library |
| item   | 90 | Copy link (items with a catalogue ID) | library |
| rating | 10 | Favourite / Undo favourite | library |
| rating | 20 | Suggest less / Undo suggest less | library |

Library state and rating come from `useItemState` / the menu's state data:
one batched `LibraryService.States` call per frame, cached briefly in Go.

New actions (add to playlist, start station, copy link…) are a new
`ActionDef` in the owning feature; album/playlist/artist targets get a menu by
rendering `ActionMenu` with that kind.

## Go events

Each module handles its own events through `eventHandlers`; payloads are
typed from the generated `Events.CustomEvents`. `app/bridge.ts` subscribes
every module's handlers (plus the app's own `app:open`), then runs each
module's `init` (initial snapshots), then calls `SystemService.FirstPaint`
after two animation frames or 300 ms, whichever comes first.

| Event | Module | Does |
| ----- | ------ | ---- |
| `auth:changed` | auth | replaces the auth query; invalidates `authInvalidations(prev, next)` |
| `sync:progress`, `sync:done` | library | updates the sync store; invalidates `syncInvalidations` / `syncDoneInvalidations` |
| `playback:state`, `playback:queue` | player | updates the playback mirror; a new track invalidates recently played |
| `settings:changed` | settings | replaces the settings query; applies appearance |
| `discover:home` | home | replaces the Home query with Go's refreshed shelves |
| `app:open` | app | `routeForOpen` → navigate (album, song, artist, playlist, music video, station, curator) |

Invalidation plans stay pure functions next to their module
(`features/auth/invalidations.ts`, `features/library/invalidations.ts`,
`app/open.ts`), each with a test.

Shortcuts work the same way: the shell runs one keydown listener that skips
handled keys, Alt/Meta, and keys inside open menus, dialogs and listboxes,
then gives the key to the first matching `Shortcut` (and prevents its
default). The search field's "/" and Ctrl+F belong to the field itself (the
search module's `TopBarSearch` slot): ↑/↓ move through its suggestions
(an ARIA combobox), Enter opens the active one or searches, Esc closes the
list, and ↓ with the list closed moves into the results.

## Boundaries

Enforced by `no-restricted-imports` in `frontend/eslint.config.js`:

- `src/features/**` may import `@/shared/**`, `@bindings/**`, packages,
  relative files inside the feature, and other features **only** as
  `@/features/<name>` (their `index.ts`). Deep paths
  (`@/features/library/queries`), `@/app/**`, and relative paths that leave
  the feature (`../library`, `../../shared/…`) are errors.
- `src/shared/**` must not import `@/features/**` or `@/app/**` (by alias or
  relative path). Where shared UI needs feature state it defines a seam the
  feature fills: `NowPlayingContext` (player's `Provider` slot), the action
  registry (modules' `actions`), the item player registry (`shared/items/play`,
  filled in module `init`), and props (the Toaster's `theme`).
- `src/app/**` may import anything.

Feature-to-feature imports must stay acyclic. Today: player → settings →
auth; home → auth, discover, library, player; discover → player; search → auth, player; album and
playlist → library, player; artist → player; library → auth.
