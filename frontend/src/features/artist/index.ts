import { t } from "@lingui/core/macro";
import { createRoute } from "@tanstack/react-router";
import { MicVocal } from "lucide-react";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { ArtistPage } from "./ArtistPage";
import { ArtistViewPage } from "./ArtistViewPage";

const artistRoute = createRoute({ ...childRoute("/artist/$id"), component: ArtistPage });

/** One shelf of a catalogue artist in full ("See all"): `view` is Go's view name. */
const artistViewRoute = createRoute({
  ...childRoute("/artist/$id/view/$view"),
  component: ArtistViewPage,
});

const routes = [artistRoute, artistViewRoute] as const;

/**
 * An artist's page (library or catalogue) with its see-all lists, and "Go
 * to artist" on song rows.
 */
export const artistModule: FeatureModule<(typeof routes)[number]> = {
  id: "artist",
  routes,
  actions: [
    {
      id: "artist.go-to",
      target: "song",
      label: () => t`Go to artist`,
      icon: MicVocal,
      group: "item",
      order: 20,
      isAvailable: (song) => !!song.artistId,
      run: (song, { navigate }) =>
        void navigate({ to: "/artist/$id", params: { id: song.artistId } }),
    },
  ],
};
