/**
 * Query keys. Every key starts with its domain so Go events can invalidate
 * a whole domain by prefix (each module's invalidations.ts).
 */

export type LibraryKind = "songs" | "albums" | "artists" | "playlists";

export const keys = {
  auth: ["auth"] as const,
  settings: ["settings"] as const,
  diagnostics: ["diagnostics"] as const,
  /** Settings → Storage: cache and database sizes. */
  storage: ["storage"] as const,
  /** The update check's latest result (UpdatesService.Status, "updates:available"). */
  updates: ["updates"] as const,
  /** What this platform supports (PlatformService.Capabilities); fixed per process. */
  capabilities: ["capabilities"] as const,

  library: ["library"] as const,
  libraryCounts: ["library", "counts"] as const,
  librarySync: ["library", "sync-status"] as const,
  libraryRecent: ["library", "recent"] as const,
  /** The History page; under libraryRecent so a new song refreshes it. */
  libraryHistory: (days: number) => ["library", "recent", "history", days] as const,
  libraryList: (kind: LibraryKind, sort: string, desc: boolean) =>
    ["library", kind, sort, desc] as const,
  libraryPage: (kind: LibraryKind, sort: string, desc: boolean, page: number) =>
    ["library", kind, sort, desc, page] as const,
  libraryAlbum: (id: string) => ["library", "album", id] as const,
  libraryArtist: (id: string) => ["library", "artist", id] as const,
  libraryPlaylist: (id: string) => ["library", "playlist", id] as const,
  librarySearch: (term: string, perKind: number) => ["library", "search", term, perKind] as const,
  /** Recent searches are user data: cleared with the library on sign-out. */
  libraryRecentSearches: ["library", "recent-searches"] as const,
  libraryRecentlyAdded: (page: number) => ["library", "recently-added", page] as const,
  libraryFavorites: (page: number) => ["library", "favorite-songs", page] as const,
  libraryMadeForYou: ["library", "made-for-you"] as const,
  libraryMusicVideos: ["library", "music-videos"] as const,
  /** An item's library state and rating (shared/lib/item-state.ts). */
  itemState: (kind: string, id: string) => ["library", "state", kind, id] as const,

  catalog: ["catalog"] as const,
  catalogSearch: (term: string) => ["catalog", "search", term] as const,
  catalogSearchGroup: (term: string, group: string) =>
    ["catalog", "search", term, "group", group] as const,
  catalogSearchHints: (term: string) => ["catalog", "search-hints", term] as const,
  catalogBrowse: ["catalog", "search-browse"] as const,
  catalogProfile: (id: string) => ["catalog", "profile", id] as const,
  catalogAlbum: (id: string) => ["catalog", "album", id] as const,
  catalogArtist: (id: string) => ["catalog", "artist", id] as const,
  catalogArtistView: (id: string, view: string) => ["catalog", "artist", id, "view", view] as const,
  catalogPlaylist: (id: string) => ["catalog", "playlist", id] as const,
  catalogSong: (id: string) => ["catalog", "song", id] as const,
  catalogSongPage: (id: string) => ["catalog", "song-page", id] as const,
  catalogVideo: (id: string) => ["catalog", "music-video", id] as const,
  lyrics: (songId: string) => ["catalog", "lyrics", songId] as const,
};
