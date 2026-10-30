import { createRoute } from "@tanstack/react-router";
import { Info } from "lucide-react";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { SongPage } from "./SongPage";

/** A catalogue song's page; `id` is a catalogue song ID. */
const songRoute = createRoute({ ...childRoute("/song/$id"), component: SongPage });

/** The song page (credits, lyrics, videos) and "View credits" on song rows. */
export const songModule: FeatureModule<typeof songRoute> = {
  id: "song",
  routes: [songRoute],
  actions: [
    {
      id: "song.view-credits",
      target: "song",
      label: "View credits",
      icon: Info,
      group: "item",
      order: 15,
      isAvailable: (song) => !!song.catalogId,
      run: (song, { navigate }) =>
        void navigate({ to: "/song/$id", params: { id: song.catalogId } }),
    },
  ],
};
