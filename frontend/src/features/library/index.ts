import { t } from "@lingui/core/macro";
import { createRoute } from "@tanstack/react-router";
import {
  Clapperboard,
  Clock3,
  Disc3,
  History,
  ListMusic,
  MicVocal,
  Music,
  Star,
  UserRound,
} from "lucide-react";

import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { libraryActions } from "./actions";
import { HistoryPage } from "./HistoryPage";
import { syncDoneInvalidations, syncInvalidations } from "./invalidations";
import {
  LibraryAlbumsPage,
  LibraryArtistsPage,
  LibraryPlaylistsPage,
  LibrarySongsPage,
} from "./pages";
import { LibraryProvider } from "./provider";
import { albumSorts, listSearch, playlistSorts, songSorts } from "./search-params";
import { useSync } from "./sync";
import { SyncIndicator } from "./SyncIndicator";
import { FavoriteSongsPage, MadeForYouPage, MusicVideosPage, RecentlyAddedPage } from "./views";

export { copyLink } from "./actions";
export { useCounts } from "./queries";

const recentRoute = createRoute({
  ...childRoute("/library/recently-added"),
  component: RecentlyAddedPage,
});
const artistsRoute = createRoute({
  ...childRoute("/library/artists"),
  component: LibraryArtistsPage,
});
const albumsRoute = createRoute({
  ...childRoute("/library/albums"),
  validateSearch: listSearch(albumSorts),
  component: LibraryAlbumsPage,
});
const songsRoute = createRoute({
  ...childRoute("/library/songs"),
  validateSearch: listSearch(songSorts),
  component: LibrarySongsPage,
});
const videosRoute = createRoute({
  ...childRoute("/library/music-videos"),
  component: MusicVideosPage,
});
const madeForYouRoute = createRoute({
  ...childRoute("/library/made-for-you"),
  component: MadeForYouPage,
});
const favoritesRoute = createRoute({
  ...childRoute("/library/favourites"),
  component: FavoriteSongsPage,
});
const historyRoute = createRoute({
  ...childRoute("/library/history"),
  component: HistoryPage,
});
const playlistsRoute = createRoute({
  ...childRoute("/library/playlists"),
  validateSearch: listSearch(playlistSorts),
  component: LibraryPlaylistsPage,
});

const routes = [
  recentRoute,
  artistsRoute,
  albumsRoute,
  songsRoute,
  videosRoute,
  madeForYouRoute,
  favoritesRoute,
  historyRoute,
  playlistsRoute,
] as const;

/**
 * The library: Recently Added, Artists, Albums, Songs, Music Videos, Made
 * for You, Favourite Songs, History and Playlists; the sync mirror and indicator;
 * and the item actions every list and header uses (add to and remove
 * from the library, Favourite, Suggest Less, Copy link).
 */
export const libraryModule: FeatureModule<(typeof routes)[number]> = {
  id: "library",
  routes,
  nav: [
    {
      section: "library",
      label: () => t`Recently added`,
      icon: Clock3,
      to: "/library/recently-added",
      order: 5,
    },
    {
      section: "library",
      label: () => t`Artists`,
      icon: MicVocal,
      to: "/library/artists",
      order: 10,
    },
    { section: "library", label: () => t`Albums`, icon: Disc3, to: "/library/albums", order: 20 },
    { section: "library", label: () => t`Songs`, icon: Music, to: "/library/songs", order: 30 },
    {
      section: "library",
      label: () => t`Music videos`,
      icon: Clapperboard,
      to: "/library/music-videos",
      order: 40,
    },
    {
      section: "library",
      label: () => t`Made for you`,
      icon: UserRound,
      to: "/library/made-for-you",
      order: 50,
    },
    {
      section: "library",
      label: () => t`Favourite songs`,
      icon: Star,
      to: "/library/favourites",
      order: 60,
    },
    {
      section: "library",
      label: () => t`History`,
      icon: History,
      to: "/library/history",
      order: 65,
    },
    {
      section: "library",
      label: () => t`Playlists`,
      icon: ListMusic,
      to: "/library/playlists",
      order: 70,
    },
  ],
  actions: libraryActions,
  eventHandlers: {
    "sync:progress": (data, { invalidate }) => {
      useSync.getState().progress(data);
      invalidate(syncInvalidations(data));
    },
    "sync:done": (data, { invalidate }) => {
      useSync.getState().done(data);
      invalidate(syncDoneInvalidations);
    },
    // A rating changed outside the UI (`sona like`): every view of the
    // item shows it, and Favourite Songs refreshes. A song it added to the
    // library arrives with the sync Go starts.
    "library:item-state": (data, { qc, invalidate }) => {
      qc.setQueryData(keys.itemState(data.kind, data.id), data);
      invalidate([["library", "favorite-songs"]]);
    },
  },
  shellSlots: { Provider: LibraryProvider, SidebarFooter: SyncIndicator },
};
