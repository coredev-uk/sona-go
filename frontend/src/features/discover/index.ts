import { createRoute } from "@tanstack/react-router";
import { ChartNoAxesColumn, Compass } from "lucide-react";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { ChartRoutePage } from "./ChartPage";
import { ChartsPage } from "./ChartsPage";
import { CuratorPage, GroupingPage, MultiroomPage } from "./EditorialPage";
import { NewPage } from "./NewPage";
import { registerPlayers } from "./players";
import { RoomRoutePage } from "./RoomPage";
import { chartSearch, chartsSearch, roomSearch } from "./search-params";

const newRoute = createRoute({ ...childRoute("/new"), component: NewPage });
const roomRoute = createRoute({
  ...childRoute("/room/$id"),
  validateSearch: roomSearch,
  component: RoomRoutePage,
});
const curatorRoute = createRoute({
  ...childRoute("/curator/$id"),
  component: CuratorPage,
});
const multiroomRoute = createRoute({
  ...childRoute("/multiroom/$id"),
  component: MultiroomPage,
});
const groupingRoute = createRoute({
  ...childRoute("/grouping/$id"),
  component: GroupingPage,
});
const chartsRoute = createRoute({
  ...childRoute("/charts"),
  validateSearch: chartsSearch,
  component: ChartsPage,
});
const chartRoute = createRoute({
  ...childRoute("/charts/$chart"),
  validateSearch: chartSearch,
  component: ChartRoutePage,
});

const routes = [
  newRoute,
  roomRoute,
  curatorRoute,
  multiroomRoute,
  groupingRoute,
  chartsRoute,
  chartRoute,
] as const;

/**
 * Discover: Apple Music's editorial pages (New, rooms, multirooms,
 * groupings, curators) and Charts; registers how shared cards play songs,
 * albums and playlists.
 */
export const discoverModule: FeatureModule<(typeof routes)[number]> = {
  id: "discover",
  routes,
  nav: [
    { section: "primary", label: "Discover", icon: Compass, to: "/new", order: 15 },
    { section: "primary", label: "Charts", icon: ChartNoAxesColumn, to: "/charts", order: 17 },
  ],
  init: registerPlayers,
};

export { discoverKeys } from "./queries";
