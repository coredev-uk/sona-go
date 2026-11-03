import { t } from "@lingui/core/macro";
import { createRoute } from "@tanstack/react-router";
import { Ticket } from "lucide-react";

import { hasAccount } from "@/features/auth";
import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { ConcertsPage } from "./ConcertsPage";
import { concertsKeys } from "./queries";
import { concertsSearch } from "./search-params";

export { ArtistConcerts, ConcertsShelf } from "./Upcoming";

const concertsRoute = createRoute({
  ...childRoute("/concerts"),
  validateSearch: concertsSearch,
  component: ConcertsPage,
});

/** Whether the account was signed in at the last auth change: only a change is news. */
let wasSignedIn: boolean | undefined;

/**
 * Concerts: the shows near a city the user picks (Sona's own list of
 * cities, kept in settings), filtered by date and genre, with ticket links
 * that open in the browser; the artist pages' "Upcoming concerts" and
 * Home's shelf come from the same data.
 */
export const concertsModule: FeatureModule<typeof concertsRoute> = {
  id: "concerts",
  routes: [concertsRoute],
  nav: [
    // After Search, so the positional Ctrl+1…9 keys of the pages before it stay.
    { section: "primary", label: () => t`Concerts`, icon: Ticket, to: "/concerts", order: 25 },
  ],
  eventHandlers: {
    // The user's own artists are part of the list while signed in.
    "auth:changed": (data, { invalidate }) => {
      const now = hasAccount(data.state);
      if (wasSignedIn !== undefined && wasSignedIn !== now) invalidate([concertsKeys.all]);
      wasSignedIn = now;
    },
  },
};
