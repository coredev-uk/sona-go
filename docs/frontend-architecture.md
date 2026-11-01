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
    main.tsx              entry: appearance binding, action + shortcut registration, render
    modules.ts            the ordered list of registered feature modules
    compose.ts            routes, nav, "go to" shortcuts, slots, actions derived from modules
    router.tsx            root route + module routes → router, typed Register
    providers.tsx         QueryClient, Tooltip and Toaster providers
    bridge.ts             Go events → module handlers, module init, app:open, FirstPaint
    open.ts (+ test)      app:open link → route plan (pure)
    shortcuts.ts (+ test) the single window keydown dispatcher; the shell's own shortcuts
    shell/                AppShell (slots), Sidebar (nav + footer slots), TopBar, StatusBanner,
                          layout store (sidebar shown/hidden), useRouteMemory (saves the
                          route for "resume where you left off", debounced)
    mini.tsx              entry of the mini player window (miniplayer.html): its own playback
                          mirror and appearance from Go events, no router or modules
  shared/                 generic building blocks; never imports features or app
    module.ts             the module contract (types below)
    router.ts             rootRoute and childRoute(path)
    actions/              item action registry (single and targets[] forms, + test), ActionMenu,
                          ItemActionMenu, ContextActionMenu, songsOf (albums/playlists → songs)
    selection/            multi-select model (+ test), per-list store, useItemList (selection,
                          context menu and drag delegated from a list's root)
    dnd/                  the pointer drag-and-drop engine: beginDrag, useDropTarget (+ test)
    components/           Artwork, Backdrop, Badges (ExplicitMark, audio labels), CollectionHeader
                          (plays header.motion through MotionCover), ConfirmDialog (ConfirmHost),
                          CoverGrid, DetailStates, EmptyState, ErrorBlock, LibraryButton,
                          MotionCover (muted looping motion artwork over the still, on screen
                          only), PageHeader, PlayButtons, RatingButtons
    tracks/               TrackList, Equaliser
    items/                Item (any-kind card DTO) rendering: itemLink, play registry
                          (registerItemPlayer, playItem), ItemMenu, ItemShelf, ItemGrid,
                          WideRow (editorial banners), TrackShelf, LinkList,
                          EditorialSections (renders Section kinds; Home, Discover, Radio)
    lib/                  art, capabilities (useCapabilities: what this platform supports), errors
                          (callFailure), format, keys, queries (isLibraryId), run,
                          item-state (useItemState: batched library state + rating;
                          changeItemState, addItemsToLibrary), library-actions (rateItem,
                          addToLibrary, removeFromLibrary and their bulk forms), confirm, content-policy
                          (restricted accounts), traits, now-playing, motion (MotionContext /
                          useMotionAllowed seam, motionAt), useMediaQuery,
                          useContentVirtualizer, utils (cn)
    shortcuts/            chord parsing/display (+ test), the shortcut registry with
                          bindings, conflicts and rebinding (+ test), useShortcutBindings /
                          useBinding (live from settings), ChordKeys/BindingKeys keycaps
    brand/                SonaMark
    styles/index.css      Tailwind entry and design tokens
    testing/              helpers for tests (query-cache invalidation probe, Linux and bare
                          capability fixtures); timers, the setup file that clears a test
                          file's pending timers
    ui/                   ShadCN components
  features/<name>/        one folder per feature; public API is index.ts
    home                  For you (Apple's personal shelves, local fallback shelves)
    discover              Discover (New), rooms, multirooms, groupings, curators, Charts;
                          registers the song/album/playlist item players
    radio                 Radio (live stations, your stations, Apple's radio page), station
                          pages (/station/$id: schedule, unavailable state; a track station's
                          seed as its byline and the songs it plays first); playStation,
                          useOnAirProgramme; Create station on songs/albums/artists; the
                          station item player
    search                the top bar's search field (suggestions, recent searches), Search page
                          (Apple Music results by group, library scope, browse categories),
                          "see all" pages per group (/search/$group), profiles (/profile/$id)
    library               Recently Added, Artists, Albums, Songs, Music Videos, Made for You,
                          Favourite Songs, History (plays by day: this computer + Apple's
                          recently played, repeats folded per day), Playlists;
                          useLibraryList, sync store, SyncIndicator,
                          sync invalidations; item actions (library, Favourite, Suggest Less,
                          Copy link); shell Provider (ConfirmHost, content policy)
    album, artist, playlist  detail pages + their queries; "Go to album/artist" actions
    player                now-playing bar, queue, now-playing view (motion cover of the
                          playing album), playback mirror, lyrics (per-song session timing
                          offset, ±0.5 s; full-screen "room" lyrics over the whole window),
                          shortcuts, playSongs/shuffleSongs (from a PlayFrom {kind, id}:
                          album/playlist/artist go to Apple's play activity)/queueItems;
                          replaceQueue (Undo toast, or a confirm with setting
                          queue.replaceConfirm, over more than 3 user-added songs; also
                          used by radio and library Songs); sleep timer menu in the bar
                          (Go's PlaybackService.SetSleep) and palette commands;
                          explicit queue rows dimmed on restricted accounts;
                          AutoPlay toggle and station/AutoPlay section in the queue,
                          live radio line in place of the seek bar; mini player button;
                          exports the playback mirror and lyrics for the mini player;
                          CdmNotice: the bar's line while a song waits for the Widevine
                          CDM (snapshot noCdm: downloading with its progress, failed, or
                          ready for another press of play), linking to Settings → Playback;
                          "Preview" beside the title (PreviewMark, shared/components/Badges,
                          Apple's word; its tooltip is also its description) while the song
                          plays as its 30-second preview (snapshot preview: SONA_PREVIEW=1),
                          and TrackAnnouncer says "Now playing a preview: …"
    cdm                   the Widevine CDM Sona downloads from Google: Go's CDMService
                          mirrored from cdm:status (useCdm; CDMService.Status in init),
                          retryCdm, cdmProgress/cdmPercent, canRetryCdm, and cdmProblem
                          (the wording for every failure code in internal/playback/cdm,
                          + test); a state change refreshes Diagnostics
    motion                whether motion artwork may play (setting, reduce effects, system
                          reduced motion, Go's battery state from power:changed), given to
                          shared covers through its Provider slot
    miniplayer            the mini player window's UI (MiniPlayer) and its toggle shortcut
    equalizer             the Equaliser page (/equalizer, Ctrl+Shift+E): presets, ten band
                          faders and a preamp over the response curve (response.ts computes
                          GStreamer's equalizer-10bands response); dragging previews through
                          AudioService.PreviewEqualizer, letting go saves settings.audio.equalizer
                          through settings' useUpdateSettings; EqualizerButton for the player bar.
                          Settings → Playback links here (a route Link, no import, so settings
                          never depends on it)
    settings              Settings page and Playback/Appearance/Desktop/Shortcuts/Storage/
                          Diagnostics sections (Playback: the Widevine row (CdmRow: version,
                          source, a running download's progress bar, Retry after a failed
                          download or update check); Storage: artwork cache limit, cache and
                          database sizes, clear caches,
                          compact the database, export/import the settings file through Go's
                          portal chooser; Desktop:
                          close action, autostart, start page, resume, window size;
                          Diagnostics: checks (the Widevine CDM's state, source, version and
                          update check beside the probe's checks), Copy diagnostics → redacted bundle from Go,
                          the Updates group; Shortcuts: rebinding with conflict checks),
                          appearance store (Appearance: theme, reduce effects, motion
                          artwork), useSettings/useUpdateSettings
    commands              Ctrl+K command palette (pages, actions, library items; rank + test),
                          "?" shortcut help sheet
    auth                  Account section, sign-in reducer (+ test), useAuth/hasAccount,
                          auth invalidations (+ test)
    updates               update notice in the sidebar footer (a newer GitHub release)
    private               private listening: Go's private mode mirrored from private:changed
                          (usePrivate, setPrivate, isForced), PrivateButton for the player bar
                          (glyph, "Private" in ink on the selected tone when on, never amber;
                          "(development build)" when a development build keeps it on;
                          aria-disabled while SONA_PRIVATE=1 forces it) and the
                          "private.toggle" palette command (no default keys). Settings →
                          Playback's row (settings/PrivateRows.tsx) uses the same mirror
```

Tests sit next to their code (`*.test.ts`). Vitest runs every file in jsdom
with two setup files: `shared/testing/timers.ts` clears the timers a file
leaves pending once its tests are done (they would otherwise fire after the
window is torn down, failing the run with `window is not defined`), and
`shared/i18n/test-setup.ts` activates the English catalogue.

## The module contract

`src/shared/module.ts`:

```ts
export type RouteDef = AnyRoute;

export type NavSection = "primary" | "library" | "footer";

export interface NavItem<TPath extends string = string> {
  section: NavSection;
  label: () => string;         // translated where shown: () => t`Albums`
  icon: LucideIcon;
  /** One of the module's own route paths. */
  to: TPath;
  /** Position inside the section, ascending. */
  order: number;
  /** Active only on this exact path (For you at "/"). */
  exact?: boolean;
  /** Default keys of its "go to" shortcut (else Ctrl+1…9 / Alt+1…9 by position). */
  keys?: readonly string[];
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

export const shortcutGroups = ["playback", "view", "pages", "navigation", "general"] as const;
export type ShortcutGroup = (typeof shortcutGroups)[number];

/** "page" (default): not in text fields. "global": Ctrl/Alt/Meta chords work in
 *  text fields too. Any other name is a context; keys may repeat across contexts. */
export type ShortcutScope = "page" | "global" | (string & {});

export interface ShortcutContext {
  navigate: UseNavigateResult<string>;
  history: RouterHistory;
}

export interface Shortcut {
  id: string;                  // stable, "<module>.<action>"; settings store bindings by it
  label: () => string;         // () => t`Play or pause`
  group: ShortcutGroup;
  keys: readonly string[];     // default chords: "Space", "Ctrl+K", "Shift+ArrowRight"; [] = none
  scope?: ShortcutScope;
  keywords?: () => string;     // extra palette words, translated
  icon?: LucideIcon;
  when?: () => boolean;        // available now (key and palette)
  repeat?: boolean;            // fires again while held
  palette?: false;             // hidden from the palette
  run: (ctx: ShortcutContext) => void;
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
  `order`; `slots` follow list order; `registerModuleActions()` puts every
  module's `actions` in the registry and `registerModuleShortcuts()` puts a
  "go to" shortcut per nav item (`nav.<path>`, e.g. `nav.library.albums`),
  the shell's shortcuts and every module's `shortcuts` in the shortcut
  registry (both called once in `main.tsx`, before the first render).
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
  nav: [{ section: "primary", label: () => t`Radio`, icon: Radio, to: "/radio", order: 30 }],
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
  label: Read<K, string>;         // (target, ctx) => t`Play next`
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

/** One item of any kind: what a selection holds and a drag carries. */
export type ActionItem = { [K in ActionTargetKind]: { kind: K; target: ActionTargets[K] } }[ActionTargetKind];

// The targets[] form: one run receives every selected item, in list order.
// Listed when every selected item's kind is in `kinds`; same fields as
// ActionDef, read from (items, ctx). ctx.container carries `indices`.
export interface BulkActionDef {
  id: string;
  target: "selection";
  kinds: readonly ActionTargetKind[];
  label: ReadMany<string>;
  /* icon, filled, group, order, isAvailable, isEnabled, data (BulkMenuData), run */
}
export type ItemActionDef = AnyActionDef | BulkActionDef; // a module's `actions`

export function registerActions(defs: readonly ItemActionDef[]): void; // throws on a duplicate id
export function actionsFor<K extends ActionTargetKind>(kind: K): readonly ActionDef<K>[];
export function bulkActionsFor(items: readonly ActionItem[]): readonly BulkActionDef[];
export function describeItems(items: readonly ActionItem[]): string; // “Airbag”, "12 songs", "5 items"
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
rendering `ActionMenu` with that kind (or `ItemActionMenu` with an
`ActionItem`). A row's `ActionMenu` renders a plain button until the pointer
reaches it, then mounts the Radix menu (a press before that opens it as it
mounts), so long lists do not mount a menu per row. `ContextActionMenu` is
the same menu opened at a point (right-click, Shift+F10, the Menu key).

Bulk (`targets[]`) actions registered today: Play next, Add to queue
(player; songs, albums and playlists, their tracks in order), Add to
playlist… and Remove from playlist (playlist; the latter in an editable
playlist's list), Add to library, Remove from library (one confirmation),
Favourite / Undo favourite (library; states loaded for the whole selection
on open). `shared/actions/songs.ts` (`songsOf`) expands albums and
playlists to their songs.

## Selection, context menus and drag and drop

`TrackList` and `CoverGrid` share `shared/selection`:

- `model.ts` (+ test): the pure selection model, rows by key so a selection
  survives re-sorts and paging. Click selects one row (track lists; a tile's
  click opens it), Ctrl+click toggles, Shift+click selects anchor…row
  (Ctrl+Shift adds the range), Shift+arrows extend from the anchor, Ctrl+A
  selects all, Escape clears, a press outside the list clears.
- `store.ts`: one zustand store per list in `SelectionContext`; rows read
  their own state with selectors (`useRowSelected`, `useRowRun`), so a
  selection change re-renders only rows whose state changed and never the
  list. Selected rows carry `aria-selected`; adjacent selected track rows
  draw as one block of 12% ink (a clear step above the hover fill), a
  selected tile sits on a plate of the same tint.
- `useItemList.tsx`: click, right-click and drag handlers delegated from the
  list's root (rows carry `data-row`, no listeners of their own), the list's
  keyboard selection keys, and the open context menu. A menu or drag on a
  row inside a multi-selection acts on the whole selection.

Cover tiles take a `target: ActionItem` (instead of a menu node) for their
menu, selection and drag.

`shared/dnd/drag.ts` is the one drag mechanism: pointer events, not HTML5
DnD (WebKitGTK runs a GTK drag session for native drags that the page cannot
read during dragover). `beginDrag(e, payload, threshold)` starts it from a
pointerdown (a few pixels of travel first, 0 for a handle); elements with
`data-drop={id}` registered with `useDropTarget(id, {accepts, describe,
hover, drop})` take drops, the innermost accepting one wins; the scroll pane
under the pointer scrolls near its edges; Escape cancels. The drag image
shows the items and what a drop would do. Targets: sidebar playlists and
pinned playlists (add), the Pinned section (pin), a "New playlist" row,
the queue panel (insert at the gap via `PlaybackService.Insert`, or reorder
its own rows), and the playlist editor (reorder).

## Rendering performance

React Compiler (`babel-plugin-react-compiler` through `@rolldown/plugin-babel`
in `vite.config.ts`) memoises every component and hook at build time, so
inline callbacks and objects passed down stay stable. What it does not do:

- **Skip children.** List items whose parent re-renders as the visible range
  moves are `memo` components: `TrackRow`, `CoverTile`, `EditorRow`,
  `QueueRow`, `ArtistRow`, `PlaylistRow`.
- **Compile everything.** It silently skips a function that breaks the Rules
  of React (`pnpm exec eslint src` reports those as `react-hooks/*`), that
  sits under an `eslint-disable` for a `react-hooks` rule, or that uses syntax
  it does not support yet (`try…finally`, `x++` on a variable a closure
  captures). Skipped today: the `Account.tsx` sign-in handlers
  (`try…finally`) and the two virtualizer hooks below.
- **Memoise around TanStack Virtual.** The virtualizer keeps its identity
  while it scrolls, so a compiled component that renders from its getters
  would show stale rows. `useVirtualizer` stays inside a hook the compiler
  skips (`useContentVirtualizer`, `QueuePanel`'s `useQueueLines`) that returns
  `items` and `totalSize`; components render from those and call the
  virtualizer only in handlers and effects (`scrollToIndex`).

The playback mirror keeps `current` and `station` objects when a snapshot
leaves them unchanged, so play/pause and seeks do not re-render the track
lists that mark the playing song.

## Strings and locales

Every string the user reads (text, `aria-label`, `title`, placeholders,
toasts, menu and command labels) goes through [Lingui](https://lingui.dev)
and lands in the English catalogue `src/shared/i18n/locales/en.po`.

**Why Lingui.** Its macros compile at build time (`@lingui/vite-plugin` with
`macroTransform: true`, a native transform, so no extra Babel pass next to
React Compiler) into calls carrying a short hashed ID; production builds drop
the English source text from the code, and `.po` catalogues compile into
plain JS objects on import, so no ICU parser ships. The runtime
(`@lingui/core` + `@lingui/react`) is 11.5 kB minified, 4.3 kB gzipped
(when it was chosen); the English catalogue adds 54 kB minified, 20 kB
gzipped, most of it the text the code no longer carries. FormatJS/react-intl
was the alternative: a larger runtime (`IntlProvider` and its formatters),
and it needs its own `@formatjs/cli` precompile step to keep the ICU parser
out of the bundle.

**Writing strings.**

- JSX text: `<Trans>Signed in as {name}</Trans>`; plurals
  `<Plural value={n} one="# song" other="# songs" />` (from
  `@lingui/react/macro`). Put values in plain variables first, so the
  catalogue reads `{name}`, not `{0}`.
- Attributes and strings computed while rendering: `const { t } = useLingui()`
  (`@lingui/react/macro`), then `` t`Play ${title}` ``. The hook's `t` is a
  reactive value, so React Compiler recomputes it when the locale changes.
- Plain functions that run later (toasts in handlers, commands, helpers):
  `` t`…` ``, `plural(n, { one: "# song", other: "# songs" })` from
  `@lingui/core/macro`. Never at module scope: an import-time `t` would freeze
  the English text (`lingui/t-call-in-function`).
- Static labels in module objects and tables are thunks evaluated where they
  are shown: `NavItem.label`, `Shortcut.label` and `Shortcut.keywords` are
  `() => string` (`` label: () => t`Play or pause` ``), as are item action
  labels (`(target, ctx) => string`).
- Numbers, dates and durations use `Intl` with the active locale through
  `shared/lib/format.ts` (`formatNumber`, `longDate`, `clockTime`,
  `relativeTime`, `formatRunningTime`, `formatBytes`, `numberFormat`,
  `dateFormat`), never `toLocaleString()` without a locale.
- Go errors: `callFailure(err).message` is the translated wording for the
  services error code (`err.cause.code`, `internal/services/errors.go`),
  mapped once in `shared/lib/errors.ts`; codes without wording (`unknown`)
  keep Go's text, also available as `.detail`.
- Brand and product names stay as written but still sit inside messages, so
  a translator can place them.

`lingui/no-unlocalized-strings` (with the plugin's recommended rules) in
`eslint.config.js` fails on any string literal or JSX text that is not in a
message, except where the user never reads it: class names, keys and IDs,
union-typed props (type-aware), `cn`/`cva` arguments, and strings without
whitespace that do not start with a capital (routes, event names, CSS
tokens). `pnpm i18n:extract` rewrites the catalogues after string changes;
`pnpm i18n:check` fails when they are out of date.

**Choosing the locale** (`shared/i18n`). `initI18n()` runs before the first
render in both windows. The choice is the account's storefront language
(`AuthStatus.language`, followed live by `ShellProviders` and the mini
player) when a catalogue matches it, else the system languages
(`navigator.languages`), else English (`chooseLocale` in `locale.ts`:
exact tag, then same language). English is bundled and activates
synchronously; any other catalogue is its own chunk, loaded before the first
render. `I18nRoot` remounts the tree when the locale changes, so labels
computed outside hooks follow. There is no language setting until a second
catalogue ships.

**Pseudo-locale.** `en-XA` accents and stretches every English message
(`⟦Ƥŀåý ñêxţ⟧`), so text that bypassed Lingui stands out. `pnpm dev:pseudo`
and `pnpm build:pseudo` force it through `VITE_SONA_LOCALE=en-XA`; it is
never chosen automatically.

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
| `integrations:status` | settings | replaces the integrations status query (notifications/inhibitor availability, Discord configured/connected) shown in the Desktop and Connections rows (`IntegrationsRows.tsx`) |
| `scrobble:changed` | settings | replaces the scrobbler status query (connected, connecting, needs reconnect, listens waiting) shown in Settings → Connections (`Connections.tsx`) |
| `discover:home` | home | replaces the Home query with Go's refreshed shelves |
| `power:changed` | motion | mirrors Go's on-battery state; motion artwork stops on battery |
| `updates:available` | updates | replaces the `updates` query; the sidebar shows the notice |
| `private:changed` | private | replaces the private listening mirror (player bar, Settings row, palette command; also sent for the tray and `sona private`) |
| `cdm:status` | cdm | replaces the Widevine CDM mirror (Settings → Playback's Widevine row, Diagnostics, the player bar's waiting line); a new state, version, source or code (not a progress step) invalidates Diagnostics |
| `app:open` | app | `routeForOpen` → navigate (album, song, artist, playlist, music video, station, curator) |

Invalidation plans stay pure functions next to their module
(`features/auth/invalidations.ts`, `features/library/invalidations.ts`,
`app/open.ts`), each with a test.

### Shortcuts and commands

A shortcut is a declared command (`Shortcut` above) with a stable id and
default keys; adding one is one entry in a module's `shortcuts`. From the
registry (`shared/shortcuts/registry.ts`) the app derives:

- **Bindings.** `config.Settings.shortcuts` maps an id to a binding string
  (space-separated chords, or `none`); an id without an entry, or with an
  invalid one, keeps its defaults. `useShortcutBindings()` reads the
  settings query, so an optimistic save or `settings:changed` applies a
  rebinding at once. Chords are canonical strings (`shared/shortcuts/chord.ts`):
  modifiers in the order Ctrl, Alt, Shift, Meta; letters upper case; Shift
  is dropped for non-letter characters ("?" is one chord on every layout);
  AltGr characters are the character; Ctrl+letter on a non-Latin layout
  uses the physical Latin key.
- **Dispatch.** `app/shortcuts.ts` runs one window keydown listener, indexed
  by chord. It skips handled and composing keys and keys inside open menus,
  dialogs and listboxes. Plain chords (no Ctrl/Alt/Meta) never fire in a
  text field, and Space, Enter, arrows and paging keys never fire on a
  control that uses them; `global` modifier chords work in text fields. The
  first shortcut on the chord whose scope allows it and whose `when` holds
  runs (default prevented; held keys repeat only with `repeat`).
- **Conflicts.** Two shortcuts conflict when they share a chord in
  overlapping scopes (`page`/`global` overlap everything; two different
  contexts do not). Settings → Shortcuts records a new chord by keypress,
  asks before taking a chord another shortcut uses (that one keeps its
  other keys), resets one binding or all, and marks remaining conflicts.
- **Commands.** Ctrl+K (`features/commands`) is an ARIA combobox over a
  grouped listbox: with nothing typed, every available command by group;
  typed, the best-ranked commands (`rank.ts`: whole label, label start,
  word start, all words, substring, keywords, letters in order), then up
  to eight library items from the local index (debounced, two characters
  minimum), then "Search Apple Music for …". `?` opens the help sheet,
  generated from the registry with the current keys. Both return focus to
  where it was when they close; a chosen command runs after that.

Default keys: Space play/pause; Ctrl+→/← next/previous; Shift+→/← seek
10 s; Ctrl+↑/↓ volume; M mute; S shuffle; R repeat; F favourite; Q queue;
L lyrics; N now playing; Esc closes now playing; / or Ctrl+F search; Ctrl+K
palette; ? help; Ctrl+B sidebar; Alt+←/→ back/forward; Ctrl+1…9 the primary
pages, Alt+1…9 the library pages, Ctrl+, Settings. The search field's own
keys stay in the field (the search module's `TopBarSearch` slot): ↑/↓ move
through its suggestions (an ARIA combobox), Enter opens the active one or
searches, Esc closes the list, and ↓ with the list closed moves into the
results.

## Platform capabilities

Sona builds for Linux, macOS and Windows, but not everything works
everywhere yet (macOS and Windows have no playback engine today). Go
reports what this platform and build support through
`PlatformService.Capabilities()` (the `Capabilities` DTO in
`internal/services/platform_dto.go`), fixed for the process.

`shared/lib/capabilities.ts`:

- `useCapabilities(): Capabilities | undefined`: the query under
  `keys.capabilities` (staleTime and gcTime Infinity, prefetched in
  `app/providers.tsx` before the first render); undefined until Go answers.
- `currentCapabilities()`: the same value for code outside React, such as a
  shortcut's `when`.

The rule: a control whose capability is false is hidden, or its absence is
explained where it would be (Settings → Playback says playback isn't
available; Diagnostics shows one row instead of the CDM checks; the
Equaliser page says it isn't available). A control is never shown only to
fail, and undefined (still loading) counts as false. On Linux every
capability that works today is true, so the Linux UI is unchanged. A call
that still reaches an unsupported path fails with error code `unsupported`,
which `callFailure` words as "That isn't available on this platform."

Gated today: the player bar's volume and the volume and mute shortcuts
(`playback.volume`); the equaliser button, Settings row, page and shortcut
(`playback.equalizer`); Settings → Playback's volume, output, Sound Check,
crossfade rows (their `playback.*` fields), the idle host and Widevine
rows (`playback.engine === "cdm"`);
Settings → Desktop's close action (`tray`), start at login (`autostart`),
start in the tray (`autostart` and `tray`), track notifications
(`notifications`) and stay awake (`sleepInhibit`); the Discord section of
Connections (`discordPresence`); the settings file export and import
(`fileChooser`); motion artwork's "paused on battery" note (`powerState`);
Diagnostics' WebKitGTK rows and rendering switches (`webkitGtk`). Diagnostics
reads `DiagPlayback.engine` ("" without an engine) from its own report.

The window's title bar (`windowControls`) is data, not a gate: its
`native` ("inset" on macOS, "frame" elsewhere), `automatic` (what the
"auto" title bar style picks here), `desktop` (the desktop Sona
recognised), `style` (the title bar style the window was created with),
`start`/`end` (the desktop's window buttons, in order) and `windowMenu`.
`app/shell/window-chrome.ts` resolves them with the window's state
(`window:state`, `DesktopService.WindowState`) into the shell's
`WindowChrome` (`resolveWindowChrome`, the same rule as Go's
`windowchrome.Platform.Resolve`): `native-inset` (room at the leading edge
for macOS's buttons, a 52px band), `drawn` (Sona's own title bar with its
`WindowControls` at the leading slot, the sidebar header or the top bar
while the sidebar is hidden, and the trailing slot, the top bar's end) or
`none`. `titleBarRegion` gives the sidebar header and the top bar the drag
region (`--wails-draggable: drag`; `index.css` gives every control inside a
`[data-title-bar]` `no-drag`), the double click (maximise) and the right
click (the desktop's window menu). The mini player reads `windowControls`
for the side of its own buttons.

Adding a capability: a field on `Capabilities` (or `PlaybackCapabilities`)
in `internal/services/platform_dto.go`, filled from the Go platform seam for
each OS (true on Linux wherever the feature works), regenerate the bindings
(`gen`), then read it with `useCapabilities()` in the control that needs it,
with a test showing the control hidden when false and shown when true
(`shared/testing/capabilities.ts` has Linux and bare fixtures).

## Boundaries

Enforced by `no-restricted-imports` in `frontend/eslint.config.js`:

- `src/features/**` may import `@/shared/**`, `@bindings/**`, packages,
  relative files inside the feature, and other features **only** as
  `@/features/<name>` (their `index.ts`). Deep paths
  (`@/features/library/queries`), `@/app/**`, and relative paths that leave
  the feature (`../library`, `../../shared/…`) are errors.
- `src/shared/**` must not import `@/features/**` or `@/app/**` (by alias or
  relative path). Where shared UI needs feature state it defines a seam the
  feature fills: `NowPlayingContext` (player's `Provider` slot), `MotionContext`
  (motion's `Provider` slot), the action registry (modules' `actions`), the item
  player registry (`shared/items/play`, filled in module `init`), and props (the
  Toaster's `theme`).
- `src/app/**` may import anything.

Feature-to-feature imports must stay acyclic. Today: player → settings →
auth; player → equalizer → settings; player → private; settings → private;
player → cdm; settings → cdm;
home → auth, discover, library, player; discover → player; search → auth, player; album and
playlist → library, player; artist → player; library → auth; commands → search;
motion → settings.

No two files or folders may differ only by letter case, including module names
without their extension (`Appearance.tsx` beside `appearance.ts`): on macOS and
Windows `./Appearance` resolves to the wrong file. `frontend/scripts/check-path-case.mjs`
checks every tracked path in CI.
