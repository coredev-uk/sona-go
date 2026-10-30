import { t } from "@lingui/core/macro";
import { createRoute } from "@tanstack/react-router";
import { Play, Radio } from "lucide-react";

import type { AnyActionDef } from "@/shared/actions/registry";
import { registerItemPlayer } from "@/shared/items/play";
import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { createStation, playStation } from "./commands";
import { radioKeys } from "./queries";
import { RadioPage } from "./RadioPage";
import { StationPage } from "./StationPage";

export { playStation } from "./commands";
export { useOnAirProgramme } from "./on-air";
export { slotLabel } from "./schedule";

const radioRoute = createRoute({ ...childRoute("/radio"), component: RadioPage });
/** A station or radio episode; `id` is its "ra.…" catalogue ID. */
const stationRoute = createRoute({ ...childRoute("/station/$id"), component: StationPage });

const routes = [radioRoute, stationRoute] as const;

const actions: readonly AnyActionDef[] = [
  {
    id: "radio.play-station",
    target: "station",
    label: () => t`Play`,
    icon: Play,
    group: "queue",
    order: 10,
    isEnabled: (s) => s.playable,
    run: (s) => void playStation(s.id),
  },
  {
    id: "radio.create-station",
    target: "song",
    label: () => t`Create station`,
    icon: Radio,
    group: "queue",
    order: 30,
    isAvailable: (s) => !!s.catalogId,
    run: (s) => void createStation("song", s.catalogId, s.name),
  },
  {
    id: "radio.create-station-album",
    target: "album",
    label: () => t`Create station`,
    icon: Radio,
    group: "queue",
    order: 30,
    isAvailable: (a) => !!a.catalogId,
    run: (a) => void createStation("album", a.catalogId, a.name),
  },
  {
    id: "radio.create-station-artist",
    target: "artist",
    label: () => t`Create station`,
    icon: Radio,
    group: "queue",
    order: 30,
    isAvailable: (a) => !!a.catalogId,
    run: (a) => void createStation("artist", a.catalogId, a.name),
  },
];

/** The station that last fed the queue, to notice a new one. */
let lastStation = "";

/**
 * Radio: the Radio page (live stations, the user's stations, Apple's
 * editorial radio), station pages, playing stations from any card, and
 * Create Station on songs, albums and artists.
 */
export const radioModule: FeatureModule<(typeof routes)[number]> = {
  id: "radio",
  routes,
  nav: [{ section: "primary", label: () => t`Radio`, icon: Radio, to: "/radio", order: 18 }],
  actions,
  eventHandlers: {
    // Apple's station history changes once a new station starts playing.
    "playback:state": (data, { invalidate }) => {
      const id = data.station?.id ?? (data.current?.live ? data.current.id : "");
      if (id && id !== lastStation) invalidate([radioKeys.recent]);
      lastStation = id;
    },
  },
  init: () => {
    registerItemPlayer("station", (item) => void playStation(item.id));
  },
};
