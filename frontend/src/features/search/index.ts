import { createRoute } from "@tanstack/react-router";
import { Search } from "lucide-react";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { GroupPage } from "./GroupPage";
import { ProfilePage } from "./ProfilePage";
import { SearchField } from "./SearchField";
import { SearchPage } from "./SearchPage";
import { groupSearch, searchSearch } from "./search-params";

const searchRoute = createRoute({
  ...childRoute("/search"),
  validateSearch: searchSearch,
  component: SearchPage,
});

/** "See all" for one Apple Music result group. */
const groupRoute = createRoute({
  ...childRoute("/search/$group"),
  validateSearch: groupSearch,
  component: GroupPage,
});

/** An Apple Music listener's profile (found through search). */
const profileRoute = createRoute({ ...childRoute("/profile/$id"), component: ProfilePage });

const routes = [searchRoute, groupRoute, profileRoute] as const;

/**
 * Search: Apple Music results by group with "see all" pages, the library
 * scope, browse categories and recent searches, profiles, and the top
 * bar's search field with suggestions.
 */
export const searchModule: FeatureModule<(typeof routes)[number]> = {
  id: "search",
  routes,
  nav: [{ section: "primary", label: "Search", icon: Search, to: "/search", order: 20 }],
  shellSlots: { TopBarSearch: SearchField },
};
