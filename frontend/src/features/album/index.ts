import { createRoute } from "@tanstack/react-router";
import { Disc3 } from "lucide-react";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { AlbumPage } from "./AlbumPage";
import { albumSearch } from "./search-params";

const albumRoute = createRoute({
  ...childRoute("/album/$id"),
  validateSearch: albumSearch,
  component: AlbumPage,
});

/** An album's page (library or catalogue), and "Go to album" on song rows. */
export const albumModule: FeatureModule<typeof albumRoute> = {
  id: "album",
  routes: [albumRoute],
  actions: [
    {
      id: "album.go-to",
      target: "song",
      label: "Go to album",
      icon: Disc3,
      group: "item",
      order: 10,
      isAvailable: (song) => !!song.albumId,
      run: (song, { navigate }) =>
        void navigate({ to: "/album/$id", params: { id: song.albumId } }),
    },
  ],
};
