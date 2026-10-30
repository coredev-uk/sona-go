import { t } from "@lingui/core/macro";
import { createRoute } from "@tanstack/react-router";
import { Sparkles } from "lucide-react";

import { discoverKeys } from "@/features/discover";
import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { HomePage, HomeShelfPage } from "./HomePage";

const homeRoute = createRoute({ ...childRoute("/"), component: HomePage });
const shelfRoute = createRoute({ ...childRoute("/for-you/$id"), component: HomeShelfPage });

const routes = [homeRoute, shelfRoute] as const;

/** For you: Apple's personal shelves (Home), with Sona's local shelves as the fallback. */
export const homeModule: FeatureModule<(typeof routes)[number]> = {
  id: "home",
  routes,
  nav: [
    {
      section: "primary",
      label: () => t`For you`,
      icon: Sparkles,
      to: "/",
      order: 10,
      exact: true,
    },
  ],
  eventHandlers: {
    "discover:home": (page, { qc }) => qc.setQueryData(discoverKeys.home, page),
    "auth:changed": (_, { invalidate }) => invalidate([discoverKeys.all]),
  },
};
