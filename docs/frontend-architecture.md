# Frontend architecture

The frontend (`frontend/src`) is a shell plus feature modules. A feature is a
folder under `src/features/` whose `index.ts` exports one module object; the
app composes the router, the sidebar, Go event handling, shortcuts, shell
slots and item actions from the ordered list in `src/app/modules.ts`. Adding
an Apple Music page means adding a folder and one line in that list.

Visual direction is `docs/design.md`. beta.music.apple.com is a reference for
_functionality_ only (pages, data, actions); every page is designed under
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
                          ItemActionMenu, ContextActionMenu, tracksOf (albums/playlists → songs)
    selection/            multi-select model (+ test), per-list store, useItemList (selection,
                          context menu and drag delegated from a list's root)
    dnd/                  the pointer drag-and-drop engine: beginDrag, useDropTarget (+ test)
    components/           Artwork, Backdrop, Badges (ExplicitMark, audio labels), CollectionHeader
                          (plays header.motion through MotionCover, round for artists),
                          ConfirmDialog (ConfirmHost), CoverGrid (a CoverItem.motion tile plays
                          through TileMotion), DetailStates, EmptyState, ErrorBlock, LibraryButton,
                          MotionCover (muted looping motion artwork over the still, on screen
                          only, from a data: URL; logs each video's outcome), TileMotion (a
                          tile's motion on hover or focus only, one tile at a time), PageHeader,
                          PersonAvatar (a person's picture or initials, decorative; lib/avatar.ts
                          has the initials and tint rules), PlayButtons, RatingButtons
    tracks/               TrackList, Equaliser
    items/                Item (any-kind card DTO) rendering: itemLink, play registry
                          (registerItemPlayer, registerVideoPlayer, canPlay, canPlayVideo,
                          playItem), ItemMenu, ItemShelf, ItemGrid,
                          WideRow (editorial banners), TrackShelf, LinkList,
                          EditorialSections (renders Section kinds; Home, Discover, Radio)
    lib/                  art (artUrl: the one place that sizes artwork requests from the rendered
                          box, density and the artwork's shape; artAspect), capabilities
                          (useCapabilities: what this platform supports), errors
                          (callFailure), format, go-events (shareEventDispatch: Go's events
                          reach every copy of the Wails runtime), keys, queries (isLibraryId), run,
                          item-state (useItemState: batched library state + rating;
                          changeItemState, addItemsToLibrary), library-actions (rateItem,
                          addToLibrary, removeFromLibrary and their bulk forms), confirm, content-policy
                          (restricted accounts), traits, now-playing, motion (MotionContext /
                          useMotionState seam, motionAt, the video outcome log, the delegated
                          tile hover listeners), useMediaQuery,
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
    home                  For you: Apple's personal shelves set by plan.ts (the lead, shelves
                          in full, the index of the rest; Section.source marks Recently
                          Played), ItemRows (compact item rows, one Tab stop), local
                          fallback shelves
    discover              Discover (New: the featured cards as a front page whose lead rotates
                          (`useLeadRotation`), the contents,
                          the shelves in bands by what they hold), rooms, multirooms,
                          groupings, curators, Charts; registers the song/album/playlist
                          and music video item players
    radio                 Radio (the live stations as a now-and-next board from their
                          schedules, the local broadcasters, then Apple's radio page in
                          bands: stations with your own first, shows and episodes, browse),
                          station pages (/station/$id: schedule, unavailable state; a track
                          station's seed as its byline and the songs it plays first; only
                          for stations with Station.hasPage: a local broadcaster plays
                          when opened, see shared/items openWithoutPage);
                          playStation, useOnAirProgramme; Create station on songs/albums/
                          artists; the station item player
    search                the top bar's search field (suggestions, recent searches), Search page
                          (Apple Music results by group, library scope, browse categories),
                          "see all" pages per group (/search/$group), profiles (/profile/$id)
    concerts              Concerts (/concerts, after Search in the sidebar): the shows near the
                          chosen city as an agenda by the venue's own days (dates.ts: the day,
                          month and clock are read in each show's IANA zone, the date presets
                          are the user's calendar days, miles where the locale uses them),
                          filtered by date, genre and "your artists" (search params when,
                          from, to, genre, yours); the city is chosen in LocationDialog (a
                          search over Go's own list: ConcertsService.Places; the choice saves
                          through ConcertsService.SetLocation, which announces
                          settings:changed and every concerts query is asked again);
                          ConcertRow (the portrait, the line-up linking to the artist, the
                          venue, the start, the distance; "Get tickets" opens the seller's
                          https link in the browser, a menu for several sellers; on an
                          artist's page the venue leads); useRoving makes the agenda one Tab
                          stop (↑/↓ rows, ←/→ a row's link and tickets). Exports
                          ArtistConcerts (the artist page's "Upcoming concerts", nothing
                          when there are none) and ConcertsShelf (Home's concerts section,
                          Section.kind "concerts"); concerts links in Apple's editorial
                          (Item kind "concerts") open the page
    library               Recently Added, Artists (list and detail side by side, ?artist=),
                          Albums, Songs, Music Videos,
                          Favourite Songs, History (plays by day: this computer + Apple's
                          recently played, repeats folded per day), Playlists;
                          useLibraryList, sync store, SyncIndicator,
                          sync invalidations; item actions (library, Favourite, Suggest Less,
                          Copy link); shell Provider (ConfirmHost, content policy)
    album, artist, playlist  detail pages + their queries; "Go to album/artist" actions;
                          the album page lays out by its content's width (sleeve and
                          listing side by side from 860px, `features/album/layout.ts`),
                          takes its colours from the cover (`features/album/palette.ts`:
                          Apple's ArtworkColors and Go's dominant colours, worked out per
                          theme and contrast-checked; the sleeve redeclares the tokens,
                          the accent replaces amber on the page) and groups a classical
                          album's movements under their works (TrackList `heading`/`title`);
                          the artist page too (header and lead columns from 900px, the
                          release lists, its bands: `features/artist/artist.ts`)
    player                now-playing bar, queue (QueueList: beside the page in QueuePanel,
                          and as the view's Up next), now-playing view (the cover, sized
                          from its measured box, and the song's label copy beside one
                          pane, `ui.ts` `pane`: the lyric sheet or Up next; motion cover
                          of the playing album), playback mirror, lyrics (LyricsService: the best
                          of the sources in Settings → Playback → Lyrics, refetched when
                          those settings change; per-song session timing offset, ±0.5 s;
                          word by word where the lyrics are word-timed (useWordSync),
                          background vocals, duet voices, romanisation and translation
                          under each line (LyricBody);
                          full-screen "room" lyrics over the whole window),
                          the controls Go says apply (`controls` in the playback state, read
                          through `availability.ts`: `useUnavailable`, `unavailableNow`,
                          `refuse`; a live station turns off Previous, Next, Shuffle, Repeat,
                          seek, AutoPlay and queueing, each control `IconButton`
                          `unavailable`: aria-disabled with its reason),
                          shortcuts, playSongs/shuffleSongs (from a PlayFrom {kind, id}:
                          album/playlist/artist go to Apple's play activity)/queueItems;
                          replaceQueue (Undo toast, or a confirm with setting
                          queue.replaceConfirm, over more than 3 user-added songs; also
                          used by radio and library Songs); sleep timer menu in the bar
                          (Go's PlaybackService.SetSleep) and palette commands;
                          explicit queue rows dimmed on restricted accounts;
                          AutoPlay toggle and the station/AutoPlay part of the queue
                          (its own list under its heading, queue-lines.ts; AutoPlay rows
                          set back in grey and never moved, but removable and playable),
                          queue rows' menu (QueueRowMenu.tsx: Play, Play next, Move to end,
                          Remove; menuMoves in queue-focus.ts leaves out a no-op; Go's
                          MoveNext/MoveToEnd; the panel announces a landed move),
                          live radio line in place of the seek bar (the song a local
                          broadcaster has on air, its cover in the bar and Now Playing,
                          snapshot onAir/broadcaster); mini player button;
                          exports the playback mirror and the sung line (SungLine) for the mini player;
                          CdmNotice: the bar's line while a song waits for the Widevine
                          CDM (snapshot noCdm: downloading with its progress, failed, or
                          ready for another press of play), linking to Settings → Playback;
                          "Preview" beside the title (PreviewMark, shared/components/Badges,
                          Apple's word; its tooltip is also its description) while the song
                          plays as its 30-second preview (snapshot preview: SONA_PREVIEW=1),
                          and TrackAnnouncer says "Now playing a preview: …";
                          music videos: playVideos (TrackRef kind "music-video"), the video
                          view (VideoView: the picture over the whole window, its frame
                          loop and renderer in video-frames.ts and video-render.ts; see
                          "Music videos" below), video rows in the queue, "Show video" in
                          the bar and Now Playing
    cdm                   the Widevine CDM Sona downloads from Google: Go's CDMService
                          mirrored from cdm:status (useCdm; CDMService.Status in init),
                          retryCdm, cdmProgress/cdmPercent, canRetryCdm, and cdmProblem
                          (the wording for every failure code in internal/playback/cdm,
                          + test); a state change refreshes Diagnostics
    motion                whether and how motion artwork plays (setting, reduce effects, system
                          reduced motion, Go's power source from power:changed, the codecs
                          WebKit decodes: H.264, else HEVC), given to shared covers through
                          its Provider slot
    miniplayer            the mini player window's UI (MiniPlayer) and its toggle shortcut
    equalizer             the Equaliser page (/equalizer, Ctrl+Shift+E): presets, ten band
                          faders and a preamp over the response curve (response.ts computes
                          GStreamer's equalizer-10bands response); dragging previews through
                          AudioService.PreviewEqualizer, letting go saves settings.audio.equalizer
                          through settings' useUpdateSettings; EqualizerButton for the player bar.
                          Settings → Playback links here (a route Link, no import, so settings
                          never depends on it)
    settings              Settings page (a route, /settings?section=…; see "Settings" below)
                          and its General/Playback/Appearance/Connections/Shortcuts/
                          Storage/Diagnostics sections (General: starting Sona, the window,
                          while music plays, updates; Playback: the Widevine row (CdmRow:
                          version, source, a running download's progress bar, Retry after a
                          failed download or update check), Sound, Listening, Advanced;
                          Storage: artwork cache limit, cache and database sizes, clear caches,
                          compact the database, export/import the settings file through Go's
                          portal chooser; Diagnostics: checks (the Widevine CDM's state,
                          source, version and update check beside the probe's checks; Motion
                          artwork: every input, the codecs, power source, last video and
                          download), Copy diagnostics → redacted bundle from Go (with the web
                          view's MotionService.Report); Shortcuts: rebinding with conflict
                          checks), appearance store (Appearance: theme, reduce effects, motion
                          artwork), useSettings/useUpdateSettings
    commands              Ctrl+K command palette (pages, actions, library items; rank + test),
                          "?" shortcut help sheet
    auth                  the top bar's AccountMenu (avatar: where the account stands, a dot
                          when it needs the user; storefront, restricted content, offline with
                          Check now, the keyring's Try again, private listening, Sign out with
                          its confirmation) and the account dialog it opens (AccountSection:
                          the native sign-in form, browser and token fallbacks, keyring
                          notices; signed in, the account's details), the palette's "Account
                          settings", sign-in reducer (+ test), useAuth/hasAccount, auth
                          invalidations (+ test)
    updates               update notice in the sidebar footer (a newer GitHub release)
    private               private listening: Go's private mode mirrored from private:changed
                          (usePrivate, setPrivate, isForced), PrivateButton for the player bar
                          (glyph, "Private" in ink on the selected tone when on, never amber;
                          "(development build)" when a development build keeps it on;
                          aria-disabled while SONA_PRIVATE=1 forces it) and the
                          "private.toggle" palette command (no default keys). Settings →
                          Playback's row (settings/PrivateRows.tsx) uses the same mirror
    onboarding            the first-run setup, a modal over the live app (the `Layer` slot,
                          SetupLayer): a 640px card sized to its step that scrolls inside
                          when the window is short; the sidebar, top bar and player bar stay
                          drawn behind the scrim and are inert. Its head is the mark, "Set
                          up Sona", "3 of 7" and "Skip setup", over a strip of seven
                          segments, each named under it (StepProgress: one Tab stop, ←/→ and Home/End move,
                          Enter goes); each step is a title and one line, Back and the way
                          on along the foot. Escape asks first (the shared confirm; on
                          Done it is Done); a click outside does nothing. The steps draw
                          their own parts, saving
                          through the same queries as Settings: choice cards (audio quality,
                          the theme pictures, the close action), short switch rows (Sound
                          Check, crossfade, private listening, reduce effects, motion
                          artwork, start at login, notifications, stay awake), a compact
                          Widevine status from the cdm mirror, and Last.fm, ListenBrainz and
                          Discord rows using settings' connection flows (useConnectLastFM,
                          LastFMWaiting, ListenBrainzForm). Apple Music reuses auth's
                          AccountSection with `actions`: an element in the setup's foot
                          (SetupFoot context) its password form portals Sign in into, as the
                          step's primary beside "Not now" (a `form=` submit); a sign-in
                          asked for while the setup is open goes to that step instead of
                          the account dialog. What the platform
                          can't do is named in one line at the foot of Desktop. Every step
                          can be skipped; Done or "Skip setup" saves
                          settings.onboarding.completed. /welcome?step=welcome|account|
                          playback|connections|appearance|desktop|done opens the setup at
                          that step over For you and redirects to /, so Go's start route
                          (services.StartRoute: no completed flag and no account in SQLite)
                          is unchanged. Settings → General's SetupRow opens it over Settings
                          at Welcome through shared/lib/setup.ts (useSetupOpen, openSetup),
                          so settings never imports onboarding
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
  label: () => string; // translated where shown: () => t`Albums`
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

export type GoEventHandler<E extends GoEventName> = (
  data: GoEventData<E>,
  ctx: EventContext,
) => void;
export type GoEventHandlers = { [E in GoEventName]?: GoEventHandler<E> };

export const shortcutGroups = [
  "playback",
  "view",
  "pages",
  "navigation",
  "general",
] as const;
export type ShortcutGroup = (typeof shortcutGroups)[number];

/** "page" (default): not in text fields. "global": Ctrl/Alt/Meta chords work in
 *  text fields too. Any other name is a context; keys may repeat across contexts. */
export type ShortcutScope = "page" | "global" | (string & {});

export interface ShortcutContext {
  navigate: UseNavigateResult<string>;
  history: RouterHistory;
}

export interface Shortcut {
  id: string; // stable, "<module>.<action>"; settings store bindings by it
  label: () => string; // () => t`Play or pause`
  group: ShortcutGroup;
  keys: readonly string[]; // default chords: "Space", "Ctrl+K", "Shift+ArrowRight"; [] = none
  scope?: ShortcutScope;
  keywords?: () => string; // extra palette words, translated
  icon?: LucideIcon;
  when?: () => boolean; // available now (key and palette)
  repeat?: boolean; // fires again while held
  palette?: false; // hidden from the palette
  run: (ctx: ShortcutContext) => void;
}

export interface ShellSlots {
  /** Wraps the shell (a context shared components read). */
  Provider?: ComponentType<{ children: ReactNode }>;
  /** Beside the page, right of it (the queue); renders null when closed. */
  Aside?: ComponentType;
  /** Covers the page column; the page is inert and hidden while `useOpen` is true. */
  overlay?: { Component: ComponentType; useOpen: () => boolean };
  /** Across the bottom of the window (the now-playing bar). */
  Bottom?: ComponentType;
  /** Over the whole window, after everything else (the first-run setup); portals itself. */
  Layer?: ComponentType;
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
  with `nav.library` (when non-empty), then the `SidebarSection` slots, then
  the `SidebarFooter` slots and `nav.footer`. The playlist module's section
  (`features/playlist/PlaylistSidebar.tsx`) draws, in order, Pinned, Made for
  you, Apple Music and Playlists. `tree.ts` decides which a library playlist
  belongs to (`playlistSection`: a catalog ID starting `pl.pm-` is a personal
  mix, any other playlist with `canEdit` false is Apple Music, the rest are
  the user's own) and builds the lists (`sidebarPlaylists`: pinned playlists
  are in none of them, folders hold only the user's own playlists and count
  the ones listed elsewhere). Made for you and Apple Music fold shut, the
  state kept in `localStorage` (`sona.sidebar.collapsed`). There is no Made
  for you page: `/library/made-for-you` redirects to Home.
- `app/shell/AppShell.tsx`: wraps the layout in every `Provider` slot, puts
  `Aside` slots beside `main#content`, `overlay` components over the page
  column (the page is `inert` and `invisible` while any overlay's `useOpen()` is
  true: WebKit paints a covered page beneath the overlay, and each frame of the
  overlay's motion cover repainted it), and
  `Bottom` slots across the window's foot, then the `Layer` slots over all
  of it: a module's own modal that covers the sidebar, top bar and player
  bar too (the first-run setup), with the window behind it inert while it
  is open. There is no full-window route: every page has the whole shell.

## Adding a feature (worked example)

A "Radio" page with a sidebar link and a Go event:

```ts
// src/features/radio/index.ts
import { createRoute } from "@tanstack/react-router";
import { Radio } from "lucide-react";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { RadioPage } from "./RadioPage";

const radioRoute = createRoute({
  ...childRoute("/radio"),
  component: RadioPage,
});

export const radioModule: FeatureModule<typeof radioRoute> = {
  id: "radio",
  routes: [radioRoute],
  nav: [
    {
      section: "primary",
      label: () => t`Radio`,
      icon: Radio,
      to: "/radio",
      order: 30,
    },
  ],
  // Hypothetical event: any key of the generated Events.CustomEvents works.
  // eventHandlers: { "radio:changed": (data, { invalidate }) => invalidate([["catalog", "radio"]]) },
};
```

```ts
// src/app/modules.ts — the one line
import { radioModule } from "@/features/radio";
export const modules = [homeModule, searchModule, radioModule /* … */] as const;
```

Inside `RadioPage.tsx`, use `getRouteApi("/radio")` for typed search/params,
shared components from `@/shared/…`, and other features only through their
index (`import { playSongs } from "@/features/player"`). Search-param
schemas live in the feature (`search-params.ts`) and are passed to
`validateSearch`. Nothing outside the feature folder and `modules.ts` changes.

## Item action registry

`src/shared/actions/registry.ts` — row/item menus list registered actions.

```ts
export interface ActionTargets {
  song: Song;
  album: Album;
  playlist: Playlist;
  artist: Artist;
  station: Station;
  "music-video": MusicVideo; // only where videos play (canPlayVideo)
}
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
export interface ActionContainer {
  kind: string;
  id: string;
  index: number;
  editable: boolean;
}

export interface ActionContext<K extends ActionTargetKind> {
  navigate: UseNavigateResult<string>;
  data<V>(data: MenuData<K, V>): V | undefined; // undefined until loaded
  container?: ActionContainer; // e.g. "Remove from this playlist"
}

type Read<K, R> = (target: ActionTargets[K], ctx: ActionContext<K>) => R;

export interface ActionDef<K extends ActionTargetKind> {
  id: string; // unique, e.g. "player.play-next"
  target: K;
  label: Read<K, string>; // (target, ctx) => t`Play next`
  icon: LucideIcon;
  filled?: Read<K, boolean>; // filled icon (a favourite's star)
  group: ActionGroup;
  order: number; // within the group
  isAvailable?: Read<K, boolean>; // listed at all (default true)
  isEnabled?: Read<K, boolean>; // greyed out when false (default true)
  disabledReason?: Read<K, string | undefined>; // greyed out, the reason under the label
  data?: readonly MenuData<K, unknown>[];
  run: Read<K, void>;
}

export type AnyActionDef = {
  [K in ActionTargetKind]: ActionDef<K>;
}[ActionTargetKind];

/** One item of any kind: what a selection holds and a drag carries. */
export type ActionItem = {
  [K in ActionTargetKind]: { kind: K; target: ActionTargets[K] };
}[ActionTargetKind];

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
export function actionsFor<K extends ActionTargetKind>(
  kind: K,
): readonly ActionDef<K>[];
export function bulkActionsFor(
  items: readonly ActionItem[],
): readonly BulkActionDef[];
export function describeItems(items: readonly ActionItem[]): string; // “Airbag”, "12 songs", "5 items"
export function menuSections<A extends { group: ActionGroup }>(
  actions: readonly A[],
): { group: ActionGroup; actions: A[] }[];
```

Features declare actions in their module's `actions`; the app registers them.
`<ActionMenu kind="song" target={song} name={song.name} />`
(`shared/actions/ActionMenu.tsx`) renders the "more" menu for any target:
available actions by group and order, separators between non-empty groups,
menu data loaded on open.

Registered song actions today (album, artist and playlist menus get the
library and rating rows too; stations get Copy link; music videos get Play
next and Add to queue only, and have a target at all only where videos play:
`videoTile` and `actionItemOf` check `canPlayVideo`):

| Group  | Order | Action                                                 | Module  |
| ------ | ----- | ------------------------------------------------------ | ------- |
| queue  | 10    | Play next (disabled if not playable or a live station) | player  |
| queue  | 20    | Add to queue (disabled if not playable or a live station) | player  |
| item   | 10    | Go to album (if the song has an album)                 | album   |
| item   | 20    | Go to artist (if the song has an artist)               | artist  |
| item   | 30    | Add to library (catalogue items not in the library)    | library |
| item   | 35    | Remove from library (with confirmation; not playlists) | library |
| item   | 90    | Copy link (items with a catalogue ID)                  | library |
| rating | 10    | Favourite / Undo favourite                             | library |
| rating | 20    | Suggest less / Undo suggest less                       | library |

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
(player; songs, music videos, albums and playlists, in order, a collection
as its tracks), Add to playlist… and Remove from playlist (playlist; the
latter in an editable playlist's list), Add to library, Remove from library
(one confirmation), Favourite / Undo favourite (library; states loaded for
the whole selection on open). `shared/actions/songs.ts` (`tracksOf`)
expands albums and playlists to their songs; `queueItems` turns music
videos into `kind: "music-video"` queue entries. A dragged music video
drops on the queue panel, not on a playlist or the pins.

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
its own rows; Alt+↑/↓ and the row menu's Play next and Move to end are the
keyboard ways, via `Move`, `MoveNext` and `MoveToEnd`), and the playlist editor (reorder).

## Startup code

Every page is in the startup bundle (about 1.4 MB minified, 450 kB gzip).
Splitting each page route into its own chunk (`lazyRouteComponent`) was
measured and left out: it cut the startup JS by 255 kB minified (67 kB
gzip) but not the first paint (median of 5 warm starts 685 ms before,
708 ms after; scripts run in about 110 ms either way, and the page only
starts loading about 410 ms after the process starts), while every first
visit to a page would wait for its chunk. `cmd/uiprobe -log` prints that
breakdown (CONTRIBUTING.md, "Measuring UI performance").

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

### Motion artwork

- **Where it plays.** Headers (album, playlist, artist, station) and the
  now-playing cover play while on screen. Cover tiles play only while
  hovered or focused, after a 400 ms dwell, one tile at a time; at rest a
  tile with motion renders a hidden marker and nothing else (no video,
  observer or layer). One set of document listeners decides the active
  tile (`listenForTileMotion`). A video pauses while the window is hidden
  (`visibilitychange`; WebKitGTK reports a window on another Hyprland
  workspace as hidden) and while its page is inert: under the Now Playing
  view or full-screen lyrics (which cover the page column) and behind a
  modal dialog. IntersectionObserver ignores occlusion, so without that the
  header under Now Playing kept decoding unseen. At most the now-playing
  cover, or a header and one tile, decode at once.
- **Cost.** `go run ./cmd/uiprobe -motion` measures it (CONTRIBUTING.md,
  "Measuring UI performance"): one playing video costs about a tenth of a
  core in the web process (a quarter for the now-playing cover) and about
  a quarter in the UI process, on a desktop where WebKit's web process
  paints in software.
- **Which tiles.** Every catalog album, playlist, artist and station tile:
  Go's `Album`, `Playlist`, `Artist` and `Station` carry `motion` (an
  `Item` takes its typed field's), and every request that fills tiles asks
  for `editorialVideo` (Home, Discover and rooms, charts, search, the
  artist page and its "See all", the album and playlist pages' shelves,
  the Radio page's own stations). Apple sends a square video for many
  albums, playlists and artists, for live, personal ("Your Station"), mood
  and discovery stations, but none for artist, song or genre stations, and
  only a 3:4 one for radio episodes; those tiles stay still. Library items
  have none, and wide hero cards don't play.
- **How.** MotionCover fetches the Go-served `/motion/` file and hands it to
  the `<video>` as a `data:` URL. Measured on WebKitGTK 2.54.1 with
  GStreamer 1.28.7: the app's `wails://` URL is refused outright
  (MediaError 4, no pipeline); a `blob:` URL fails or stalls on about half
  the loads (qtdemux reads past the end through byte ranges: "atom has bogus
  size") and decodes corrupt frames at each loop; a `data:` URL played 8 of 8
  loads and 20 of 20 mount cycles, and looped cleanly.
- **Sizes.** Tiles ask for 180 CSS px (the 360 px rendition at 2×), headers
  200, the now-playing cover 420 (`motionAt`, at the screen's density, at
  most 2×). H.264 always, HEVC (`?codec=hevc`) only where WebKit decodes
  HEVC but not H.264.
- **Diagnosable.** Each video's outcome (playing, download failed with Go's
  reason, MediaError, stalled: no time advance within 4 s of `playing`)
  goes to `useMotionLog`; Settings → Diagnostics shows it with every input.

### Word-timed lyrics

The lyrics column and full-screen lyrics render each timed word as a span
carrying its begin and end (`data-wb`, `data-we`, in ms) inside a
`[data-line=<index>]` element, lines grouped into `[data-verse]` blocks,
and React renders them once. The sung line comes from `useSungLine`
(`lyric-words.ts`, `sungLine`): a timer waits for the next line or rest,
so the view re-renders only when the sung line changes, never per word
and never on a polling interval. `useTurnPage` scrolls the sheet a verse
at a time when the sung line leaves the reading band.

- **The loop** (`useWordSync`) reads the playback mirror's position,
  extrapolated from Go's last snapshot (`livePosition`), minus the song's
  lyrics offset (`wordClock`: no lead, unlike the line clock), and touches
  only the lines being sung (`singingLines`: the last line begun unless
  the voice rests after it, plus earlier ones whose background vocals
  still run). On each word it sets `data-sung` ("part" or "done") and, on
  a held note being sung, `--p` (0–1); the stylesheet turns those into
  the fill. The timing maths is plain functions in `lyric-words.ts`.
- **When it runs.** Timers only, while the song plays and the window is
  visible: one at the next word boundary, line or rest (`nextChange`), or
  every 80 ms while a held note (600 ms or longer, `HELD_MS`) fills. Short
  words light whole. A new snapshot (play, pause, seek), an offset change
  or the window becoming visible restarts it from the new clock; paused,
  it draws the paused position once and stops. Under reduced motion,
  reduce effects and in the mini player (`whole`) every word lights whole.
- **Offsets** are shared between the windows over a `BroadcastChannel`
  (`lyrics.ts`, `shareOffsets`), so the mini player keeps the timing set
  in the main window.

### Performance: what playing costs

In WebKitGTK every frame the page draws costs a composite of the whole
window in the UI process, and where the web process paints in software
(NVIDIA on Wayland) a large share in the web process too, so the cost of
anything that moves is its frame rate. Measured (the lyrics view open over
Settings, a 30-second preview with placeholder word-timed lyrics in which
every word is a held note, the worst case; one 10 s sample, % of one core):

|                                                                                  | desktop web   | desktop UI    | headless web  | headless UI   |
| -------------------------------------------------------------------------------- | ------------- | ------------- | ------------- | ------------- |
| paused                                                                           | 0.1%          | 0.2%          | 0.2%          | 0.3%          |
| playing, 60 fps fill (before)                                                    | 15.1%         | 35.0%         | 18.6%         | 33.9%         |
| playing, held-note fill on timers                                                | 5.4%          | 11.6%         | 8.0%          | 15.6%         |
| full screen, before → after                                                      | 28.0% → 13.1% | 78.3% → 26.4% | 23.3% → 12.9% | 32.4% → 13.1% |
| Now Playing over Search (the playing row's equaliser underneath), before → after | 99.5% → 5.6%  | 22.7% → 8.8%  | 100% → 11.2%  | 44.2% → 11.2% |
| Search, no overlay, equaliser smooth → stepped                                   | 99.7% → 9.6%  | 29.2% → 4.8%  | 99.6% → 13.4% | 67.3% → 12.2% |

Desktop: Hyprland on NVIDIA (RTX 3080), 1276×704 window, WebKitGTK
2.54.1; headless: the sway harness at 1440×900. Before, frames with the
equaliser under Now Playing took 39 ms at the median (25 fps): the whole
app lagged while a song played. The rules that follow: nothing in Sona
runs a frame loop while music plays; a loop that must show life steps
(`steps()`) at a few changes a second; anything covered by an overlay
stops (`NowPlaying.covered`, MotionCover's `inert` check).

## Music videos

A music video is a queue entry like a song (`QueueItem.video`; its `id` is
the catalogue video's). Cards and tiles of kind `music-video` (`Item`, and
`videoTile` for the `MusicVideo` DTO: artist, album and song shelves, the
video page's own shelves, the library's Music Videos) play through the video
player discover registers (`registerVideoPlayer`), which calls the player's
`playVideos`: `PlaybackService.PlayItems` with `{ id, source, kind:
"music-video" }` refs and no PlayFrom (videos are never reported as plays),
then the video view. `canPlay`/`canPlayVideo` give them a play control only
where `Capabilities.playback.video` is true (false on Windows, in preview
mode, without an engine); elsewhere the tiles keep opening the video's page,
which says Sona can't play videos here and keeps "Watch in Apple Music". The
video page's play control is the amber disc, with a note that Sona plays
standard definition.

- **The view** (`features/player/VideoView.tsx`) is the player's fourth
  overlay, `useUI` overlay `"video"`, rendered by `NowPlayingOverlay` over
  the whole window (`fixed inset-0`, like full-screen lyrics). It opens when
  `playVideos` starts a video and when Go's state names a new video that is
  loading or playing (`playback:state` in `features/player/index.ts`), and
  again from the bar's cover or "Show video" button, Now Playing's "Show
  video" and the `player.video` shortcut (V). It closes with Escape or its
  close button (playback carries on in the bar), when the queue moves on to
  a song, or after 2 s if the play that opened it never reaches the mirror.
- **Layout.** A title strip (the video's name, linking to its page, the
  artist, the SD badge with its explanation, a status line: "Loading the
  video…" before the first picture, "Buffering…" when Go is loading or
  pictures stop coming while playing), the picture, and the transport (seek
  with elapsed and remaining time, previous, play/pause, next, the bar's
  volume control). The picture's box takes the picture's shape inside the
  stage (`fitPicture`), black behind the picture only; before the first
  picture it shows the video's artwork at 16:9. The strips fade after 3 s
  of no pointer or key while playing, and stay while paused, loading,
  failed, or while a control has keyboard focus (`:focus-visible`), so Tab
  never lands on an unseen control; reduced motion makes the fade instant.
- **Keys** (handled on the view, which takes focus when it opens and keeps
  Tab inside): Space and K play and pause (Space stays a focused button's),
  Left and Right seek 5 s (`seekBy`; a focused slider keeps its arrows,
  Shift+arrows stay the player's 10 s), F toggles the window's full screen
  (`Window.Fullscreen`/`UnFullscreen`; the view leaves full screen when it
  closes if it entered it), M mutes where Sona sets the volume, Escape
  leaves full screen, then closes. Ctrl chords and other keys reach the
  shortcut dispatcher.
- **The frame loop** (`video-frames.ts`, `runFrameLoop`): one request in
  flight at a time to `GET /video/frame?after=<seq>` (Go's
  `internal/playback/video` surface, a same-origin asset URL), aborted when
  the view unmounts. A 200 is a picture (`parseFrame`: the 16-byte "SVF1"
  header, little-endian seq, width, height and position, then the I420
  planes, `planeLayout`), and the next request waits from its
  `X-Video-Seq`. A 204 without `X-Video-Seq` timed out: the picture stays
  and the view counts a stall. A 204 with it means the surface was cleared:
  the picture goes and the next request waits from that sequence. A failed
  request waits a second and asks again. React re-renders only on a new
  size, a new queue entry or a stall.
- **Frame pacing.** Go already times pictures to the audio clock (the
  child releases each when it is due), so the web view adds no clock of its
  own: a picture is drawn at the next animation frame
  (`requestAnimationFrame`), and the next one is fetched only after that,
  so at most one picture is uploaded per display frame and none queue up.
  A picture that waited longer than `STALE_MS` (250 ms) for its animation
  frame is dropped and the same `after` asked again, which answers at once
  with the picture due now. A window that isn't drawn (minimised, on
  another workspace) gets no animation frames, so the loop stops fetching
  and uploading until it is drawn again. Most of a video's CPU cost is
  WebKitGTK compositing each new frame, not this path (docs/architecture.md
  §8, "Music videos").
- **The renderer** (`video-render.ts`): WebGL, the three planes as
  `LUMINANCE` textures (unpack alignment 1, linear filtering, clamped) and a
  fragment shader converting BT.709 limited range to RGB; a lost context is
  rebuilt and the picture redrawn. Without WebGL, a 2D canvas with the same
  conversion in JavaScript (`i420ToRgba`). The canvas is the picture's size
  (at most 864×486, SD only) and CSS scales it to the box.
- **Errors** come from `PlaybackState.error` and replace the picture with a
  block (what failed, `codeMessage`'s reason, "Watch in Apple Music" from
  the video's catalogue page, and "Try again" except for `video_hd_only` and
  `video_licence_refused`, which playing again can't change). Where videos
  don't play at all the block says so. The bar and the mini player say
  "Couldn't play this video." with the reason on hover.
- **Elsewhere.** Queue rows mark a video with a clapperboard (and "music
  video" in the row's name) and link its title to `/music-video/$id`. The
  bar, Now Playing and the mini player drop what a video lacks: lyrics
  (`lyricsSongId` is "" for a video, so nothing fetches them, and the lyrics
  shortcuts are unavailable), the album, and favouriting it as a song (the
  `F` shortcut and `toggleFavourite`). Videos have their own action kind,
  `music-video` ("Item action registry"): Play Next and Add to Queue only.

## Strings and locales

Every string the user reads (text, `aria-label`, `title`, placeholders,
toasts, menu and command labels) goes through [Lingui](https://lingui.dev)
and lands in the English catalogue `src/shared/i18n/locales/en.po`.

**Why Lingui.** Its macros compile at build time (`@lingui/vite-plugin` with
`macroTransform: true`, a native transform, so no extra Babel pass next to
React Compiler) into calls carrying a short hashed ID; production builds drop
the English source text from the code, and `.po` catalogues compile into
plain JS objects on import, so the ICU parser stays out of the main bundle
(a separate chunk loads it only for Apple's runtime translations). The runtime
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
- Go errors: Go sends codes, never text for the UI. `callFailure(err).message`
  is the translated wording for the services error code (`err.cause.code`,
  the generated `ErrorCode` enum from `internal/services/errors.go`), mapped
  once in `shared/lib/errors.ts` as a `Record<ErrorCode, …>`, so a new code
  without wording fails the type check; `codeMessage(code)` words a code a
  DTO carries (`AuthStatus.lastErrorCode`, sync events, update checks).
  `unknown`, an empty or an unrecognised code get a generic "Something went
  wrong."; Go's text is only in `.detail`, for logs. Non-error reasons have
  their own enums, worded where they show (`stationUnavailable`, the
  sandbox and realtime rows in Diagnostics). Import enums from
  `@bindings/<pkg>/models`: tests mock `@bindings/services` itself.
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
render in both windows and activates English, which is bundled, at once.
After the first paint `startTranslations()` looks for a translation: for
each preferred tag in turn, the account's storefront language
(`AuthStatus.language`, followed live by `ShellProviders` and the mini
player) and then the system languages (`navigator.languages`), up to the
first English one (`translationCandidates` in `locale.ts`), it loads the
tag's layers and shows the first locale complete enough
(`chooseTranslation`); otherwise English stays. `I18nRoot` remounts the tree
when the locale changes, or when the shown locale's catalogue changes (a
revision bumped in `revision.ts`), so labels computed outside hooks follow.

**The Language setting** (`Settings.language`, Settings → General's first
row, `LanguageRow.tsx`; also on the first-run setup's welcome step). Its
options are Automatic, English, and every locale complete enough to show:
Apple's locales from `TranslationsService.Languages(minShare)` (Go lists
them only once its mapping could reach the share Apple's layer needs,
`appleShareNeeded()`: below that it answers with the mapped share alone and
fetches nothing; a locale's coverage is counted once it is cached, else the
mapped share bounds it), with the community catalogue of the same language
added, and community catalogues Apple has no locale for (`languageOptions`
in `languages.ts`, names in each language's own words through
`Intl.DisplayNames`). The row appears once one option besides English
exists, or while a language is chosen. "translations:languages" announces
Apple's locale table when it arrives. Automatic chooses as above; a chosen
tag (`followLanguageSetting`) is shown whenever Sona has any of it, English
otherwise. Go's own text (the tray, notifications, the command line, file
dialogs: `internal/i18n/l10n`) follows the setting, else the system's
languages, and so does Apple's content: requests ask for it with `l=` when
the account's storefront offers it (`applemusic.ContentLanguage`; the
storefront's `supportedLanguageTags` are kept with the account, and cached
Apple responses are keyed by the language they were asked in). Changing
the setting refetches every query.

**Translations: Apple first.** A translated locale is built from layers,
first one wins per message:

1. Apple Music's own translation, for messages mapped to one of Apple's web
   string keys (`apple-keys.json`). It is never committed or bundled: Go
   fetches it from Apple at run time (below), and `TranslationsService.Apple(tag)`
   returns it from Go's cache, keyed by Sona's message ID, with its locale
   (Apple's, e.g. "fr-FR" for "fr-BE") and its coverage.
2. The community catalogue for the messages Apple has no string for
   (`community/<locale>.po`, bundled as on-demand chunks, matched like
   `matchLocale`: the tag, its language, or a sibling region).
3. English (`locales/en.po`).

`loadTranslation` (`index.ts`) merges them. Apple's messages arrive as ICU
text; they are compiled with `@lingui/message-utils`' `compileMessageOrThrow`,
imported only when a translation arrives, so English never loads the
parser, and a message that does not compile stays English. `lingui.config.ts`
lists only the catalogues `lingui extract` writes; Vite compiles with
`lingui.runtime.config.ts`, which adds the community catalogues and their
locales, so extraction never touches them. A locale is shown once its
layers translate at least `selectableShare` (90%) of Sona's messages:
Apple's coverage, counted by Go against the same `en.po`, plus the
community messages Apple lacks (`translatedShare`). With about 7% of Sona's
messages mapped today, no locale reaches it, and Sona shows English.

**Fetching Apple's translations** (`internal/i18n/applestrings`,
`TranslationsService`). Each `TranslationsService.Apple(tag)` call reads
Go's cache and remembers the tag. Once the network opens after the first
paint, Go finds the locale table in music.apple.com's current index script
(from the small `/us/home` page), matches each remembered tag to Apple's
locale (`MatchLocale`: the tag, the same region, script, bare language,
the language's own region), downloads that locale's bundle, converts the
mapped keys and caches the result in `<cache>/strings/<apple-tag>.json`
with the bundle's content-hashed name and the mapping's version. It checks
for a new Apple release at most once a day (`RefreshInterval`), and
re-converts after a Sona update changes the mapping; between checks and
offline the cache serves. "translations:apple" announces a locale whose
messages arrived or changed, and the frontend chooses again. Apple's input
is checked before anything is kept: size caps on each response, the bundle
parsed as data (string constants and one object literal, never run),
only mapped keys converted, placeholders checked against the mapping (an
Apple placeholder with no Sona argument, or a Sona argument missing from
Apple's text, drops the message), and a size cap per message. The text is
rendered through Lingui as text, never as HTML.

The mapping (`apple-keys.json`: msgid, context, Apple key, plural flag,
Apple → Sona placeholder names; embedded into the Go binary together with
`en.po` by `frontend/src/shared/i18n/embed.go`) is proposed by
`cmd/i18ntool match`, which matches each plain or single-plural message
against Apple's en-GB strings exactly (same text, placeholders paired by
position) and then normalised (case, curly quotes, ellipsis, dashes,
spacing, closing punctuation; plural categories must agree), and is
reviewed by hand; rejected pairs are kept in `apple-keys-rejected.json` so
they are not proposed again (CONTRIBUTING.md, "Strings and translations").
The conversion: `@@name@@` becomes `{name}` through the mapping's renames,
a plural key's `.zero`/`.one`/`.two`/`.few`/`.many`/`.other` forms become an
ICU plural over Sona's argument (Apple's count placeholder becomes `#`),
and ICU syntax characters in Apple's text are quoted. `cmd/i18ntool import`
runs the same conversion and checks over every locale and writes
gitignored `apple/<locale>.po` copies for review; the app never reads them.

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

Go delivers an event by calling `window._wails.dispatchWailsEvent`, which
each copy of `@wailsio/runtime` replaces with its own dispatcher as it
loads. Both entry points call `shareEventDispatch()`
(`shared/lib/go-events.ts`) first, which makes that property a fan-out: a
second copy loaded into a running page (`import('/wails/runtime.js')`, as
the Wails MCP server's `call_bound_method` does) gets the events too, and
no longer takes them from the app's handlers.

Events and call results reach the page by separate paths, so a snapshot a
module fetched can arrive after events Go sent later. A snapshot is
applied only if none of the events it would overwrite arrived while it was
in flight: the sync store counts `sync:progress`/`sync:done` events and
every `LibraryService.SyncStatus` answer (`syncStatusQuery`) carries the
count from before the call.

| Event                              | Module   | Does                                                                                                                                                                                                  |
| ---------------------------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `auth:changed`                     | auth     | replaces the auth query; invalidates `authInvalidations(prev, next)`                                                                                                                                  |
| `sync:progress`, `sync:done`       | library  | updates the sync store (`SyncStatus` answers too, unless an event overtook them); invalidates `syncInvalidations` / `syncDoneInvalidations`                                                           |
| `playback:state`, `playback:queue` | player   | updates the playback mirror; a new track invalidates recently played                                                                                                                                  |
| `settings:changed`                 | settings | replaces the settings query; applies appearance                                                                                                                                                       |
| `integrations:status`              | settings | replaces the integrations status query (notifications/inhibitor availability, Discord configured/connected) shown in the Desktop and Connections rows (`IntegrationsRows.tsx`)                        |
| `scrobble:changed`                 | settings | replaces the scrobbler status query (connected, connecting, needs reconnect, listens waiting) shown in Settings → Connections (`Connections.tsx`)                                                     |
| `discover:home`                    | home     | replaces the Home query with Go's refreshed shelves                                                                                                                                                   |
| `power:changed`                    | motion   | mirrors Go's power source (PowerState); motion artwork stops on battery                                                                                                                               |
| `updates:available`                | updates  | replaces the `updates` query; the sidebar shows the notice                                                                                                                                            |
| `private:changed`                  | private  | replaces the private listening mirror (player bar, Settings row, palette command; also sent for the tray and `sona private`)                                                                          |
| `library:item-state`               | library  | sets an item's library state after a change outside the UI (`sona like`); invalidates Favourite Songs                                                                                                 |
| `cdm:status`                       | cdm      | replaces the Widevine CDM mirror (Settings → Playback's Widevine row, Diagnostics, the player bar's waiting line); a new state, version, source or code (not a progress step) invalidates Diagnostics |
| `app:open`                         | app      | `routeForOpen` → navigate (album, song, artist, playlist, music video, station, curator)                                                                                                              |

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
  runs (default prevented; held keys repeat only with `repeat`). One whose
  `unavailable()` returns a reason (Next on a live station) takes the chord
  but only says the reason (`announceUnavailable`, a toast under one id, said
  once however long the key is held); the command palette leaves it out.
- **Conflicts.** Two shortcuts conflict when they share a chord in
  overlapping scopes (`page`/`global` overlap everything; two different
  contexts do not). Settings → Shortcuts records a new chord by keypress,
  asks before taking a chord another shortcut uses (that one keeps its
  other keys), resets one binding or all, and marks remaining conflicts.

These listeners only see the page's own document. WebKit's web inspector is a separate web view docked in the main
window, so its keys never reach them; typing in its console depends on GTK
giving that view the keyboard focus, which Go's focus guard
(`internal/shell/contextkey_linux.c`) leaves alone. Debugging with it:
CONTRIBUTING.md, "Debugging with the web inspector"; Settings → Diagnostics →
Developer tools (`features/settings/DeveloperTools.tsx`) turns it on in
release builds from the next start and opens it.
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

The mouse's back and forward buttons (mouse 4 and 5) go back and forward
through the same history. WebKitGTK delivers neither to the page: it makes a
button-0 `mousedown`/`mouseup`/`auxclick` of each, which cannot tell them
apart and does not navigate. So on Linux Go takes them with a GTK click
gesture on the window, in the capture phase and before the web view
(`internal/shell/mousenav_linux.c`, buttons 8 and 9, claimed so the page
never sees the press), and sends `window:navigate` (`"back"` or
`"forward"`); `app/shell/useMouseNavigation.ts` calls `history.back()` or
`history.forward()`. They are not shortcuts: a mouse button has no chord,
so they are not in the help sheet or Settings → Shortcuts.

## Settings

Settings is a page, `/settings?section=<id>` (`features/settings`), not a
dialog. Deep links (the status banner, the player bar's Widevine notice,
the shortcut help) name a section; Back and Forward leave it
like any page; the player bar stays live while you change what you hear;
the equaliser and other pages it links to open in place; dialogs inside a
section (Reset all shortcuts, Disconnect) never stack on another dialog;
and the 940×600 minimum window keeps its whole content area.

The Apple Music account is not a section: it lives in the top bar's account
menu and the account dialog (`features/auth`). Every "Sign in" action (empty
states, the status banner, the player bar) opens that dialog through
`shared/lib/account.ts` (`openAccountDialog`), so no feature depends on
auth to offer it; an old `/settings?section=account` link (a saved last
page) opens the dialog over Settings (the route's `beforeLoad`).

- **Sections**, in order: General, Playback, Appearance,
  Connections, Shortcuts, Storage, Diagnostics (`search-params.ts`). A
  section is one entry there plus its title and component in
  `Settings.tsx`. Choosing a section replaces the history entry.
  `/settings` with no section, or one that no longer exists (old links to
  `desktop`), opens the section shown last in this run, at first the first
  one, so removing a section from the list needs no other change.
- **The index** (`Settings.tsx`): the h1, Find a setting (`FindSetting.tsx`)
  and the section list, each section with its state in one line
  (`summaries.ts`, `useSectionSummaries`: settings, capabilities and the
  scrobblers' status only). Below 960px it wraps above the content and the
  section heading is visually hidden.
- **Find a setting** (`find.ts`): `settingIndex()` lists every row (its
  `data-setting` id, section, group, title, a few other words, and `when`
  for rows a section shows only in some states) and every registered
  shortcut; `findSettings` matches every word of the query, accents and
  case folded, titles that start with it first. Choosing a result
  replaces the section and calls `focusSetting(id)`, which waits up to 3 s
  for the row, focuses its control (or the row) and sets `data-found` on
  it for 2 s. `find.test.tsx` renders each section and checks that every
  entry's row is there, so a renamed or new row can't be missed.
- **Groups**: a section is a stack of groups (`SettingsSections`), each an
  h3 over its rows (`SettingsGroup`, `SettingsPanel`), with an optional
  untitled block first for a state that needs attention (Playback's
  Widevine row). Rows are `SettingRow` (`data-setting` = its id; the frame
  is `ROW_FRAME`, one line above each row): label and description on the
  left (the description is the control's `aria-describedby`), the control
  on the right; selects use `SELECT_WIDTH`, which grows with a long value
  instead of cutting it. A row the platform can't back is an
  `UnavailableRow` (title and why, a `group` named by the title), never a
  missing row. Loading draws `SettingsLoading`.
- **General** holds starting Sona (start at login, start in the tray once
  that is on, Open on with "The page you were last on" as its first
  choice), the window (close action, title bar, size), while music plays
  (track notifications, stay awake) and updates. A Language setting
  belongs at the top of General; first-run setup's "Set up Sona again" row
  is the last row of its Starting Sona group.
- **Playback**: Widevine's state first (the notice in the player bar links
  here), then Sound (quality, output, Sound Check, equaliser, crossfade),
  Listening (private listening, replacing the queue) and Advanced (the idle
  playback host).
- **Saving**: each control saves itself through `useUpdateSettings`
  (optimistic, rolled back with a toast on failure; `settings:changed`
  confirms). The volume is not a Settings row: the player bar, MPRIS and
  `sona volume` set it on the coordinator, `PlaybackService` saves it, and
  `SystemService.UpdateSettings` keeps the saved volume whatever the page
  sends, so an older copy of the settings never moves it.

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
explained where it would be (Settings keeps the row and says why, with
`UnavailableRow`; Settings → Playback says playback isn't available;
Diagnostics shows one row instead of the CDM checks; the Equaliser page
says it isn't available). A control is never shown only to fail, and
undefined (still loading) counts as false and shows neither. On Linux every
capability that works today is true, so the Linux UI is unchanged. A call
that still reaches an unsupported path fails with error code `unsupported`,
which `callFailure` words as "That isn't available on this platform."

Gated today: the player bar's volume and the volume and mute shortcuts
(`playback.volume`); the equaliser button, Settings row, page and shortcut
(`playback.equalizer`); all of Settings → Playback but its "isn't
available" note (`playback.available`), and within it the output, Sound
Check and crossfade rows (their `playback.*` fields), private listening
(`playback.privateListening`), the idle host and Widevine rows
(`playback.engine === "cdm"`); music videos' play controls and the video
view (`playback.video`: the play disc on the video page, play controls on
video tiles and cards, otherwise explained on the page and in the view);
Settings → General's close action (`tray`), start at login (`autostart`),
start in the tray (`autostart` and `tray`, shown once start at login is
on), track notifications (`notifications`) and stay awake
(`sleepInhibit`); the Discord row of Connections (`discordPresence`);
the settings file export and import
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

## Pre-release albums

`shared/lib/release.ts` reads Go's `Album.prerelease` and `releaseAt`
(local midnight on the release date): `isUpcoming` (also compared with the
clock, so an open page sees the release), `releaseBadge` ("Out 19 Nov",
Intl, locale-aware) and the long and short release days. Album tiles
(`albumTile`, `toCoverItem`) carry the badge as `CoverItem.badge`, drawn on
the cover's corner instead of a play control (`canPlay` is false for an
upcoming album) and read after the title by assistive technology. The album
page (`features/album/ReleaseStatus.tsx`) puts the countdown
(`formatTimeLeft`, re-read every minute with `shared/lib/useNow`) above its
actions: Play takes only the songs already out ("Play the 8 songs out
now"; `playSongs` skips the rest), with no Shuffle until the release,
then "Remind me" and, signed in, "Pre-add" (`LibraryButton`'s `addLabel`;
the menu says "Pre-add to library"). Each song not out yet says "Out
19 Nov" under its title, and Go refuses to play it (`not_released`). When
the release passes, the page reloads the album.
`features/reminders` mirrors Go's `RemindersService` ("reminders:changed"),
owns `RemindButton` (hidden without `Capabilities.notifications`) and the
album menu's "Remind me" / "Turn off reminder"; it sends the notification's
body ("Out now · {artist}") in the interface's language.

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
  player registry (`shared/items/play`, filled in module `init`), the account
  dialog's open state (`shared/lib/account.ts`, rendered by auth), and props (the
  Toaster's `theme`).
- `src/app/**` may import anything.

Feature-to-feature imports must stay acyclic. Today: player → settings;
auth → private; player → equalizer → settings; player → private; settings → private;
onboarding → auth, cdm, private, settings;
player → cdm; settings → cdm; concerts → auth; home and artist → concerts;
home → auth, discover, library, player; discover → player; search → auth, player; album and
playlist → library, player; artist → player; library → auth; commands → search;
motion → settings.

No two files or folders may differ only by letter case, including module names
without their extension (`Appearance.tsx` beside `appearance.ts`): on macOS and
Windows `./Appearance` resolves to the wrong file. `frontend/scripts/check-path-case.mjs`
checks every tracked path in CI.
