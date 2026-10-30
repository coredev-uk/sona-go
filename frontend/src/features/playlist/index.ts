import { createRoute } from "@tanstack/react-router";

import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { playlistActions } from "./actions";
import { editing } from "./edit";
import { PlaylistDialogsProvider } from "./PlaylistDialogs";
import { PlaylistPage } from "./PlaylistPage";
import { PlaylistSidebar } from "./PlaylistSidebar";
import { playlistKeys } from "./queries";
import { bindQueryClient } from "./track-edits";

export { addToPlaylist } from "./commands";

const playlistRoute = createRoute({ ...childRoute("/playlist/$id"), component: PlaylistPage });

/**
 * Playlists: the playlist page (catalogue and library, with editing), the
 * sidebar's pins and folder tree, the add-to-playlist picker and the
 * playlist dialogs, and the pin / add / edit / move / delete item actions.
 */
export const playlistModule: FeatureModule<typeof playlistRoute> = {
  id: "playlist",
  routes: [playlistRoute],
  actions: playlistActions,
  eventHandlers: {
    "playlists:changed": (data, { invalidate }) => {
      const stale: (readonly unknown[])[] = [];
      // A playlist being edited here keeps its optimistic state; the edit's
      // own answer replaces it.
      for (const id of data.ids) if (!editing(id)) stale.push(playlistKeys.detail(id));
      if (data.tree) stale.push(playlistKeys.tree, keys.libraryCounts, ["library", "playlists"]);
      if (data.pins) stale.push(playlistKeys.pins);
      if (stale.length) invalidate(stale);
    },
  },
  init: ({ qc }) => bindQueryClient(qc),
  shellSlots: { Provider: PlaylistDialogsProvider, SidebarSection: PlaylistSidebar },
};
