import { createRoute, redirect } from "@tanstack/react-router";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { SetupLayer } from "./SetupLayer";
import { setupSearch, showSetup } from "./steps";

/**
 * /welcome?step=… opens the setup over For you at that step; the route
 * itself is never shown. Go opens the main window on it on a first start
 * (services.StartRoute).
 */
const welcomeRoute = createRoute({
  ...childRoute("/welcome"),
  validateSearch: setupSearch,
  beforeLoad: ({ search }) => {
    showSetup(search.step);
    throw redirect({ to: "/", replace: true });
  },
});

/**
 * The first-run setup: a layer over the whole window (SetupLayer), opened
 * by its /welcome links and by Settings → General (shared/lib/setup).
 */
export const onboardingModule: FeatureModule<typeof welcomeRoute> = {
  id: "onboarding",
  routes: [welcomeRoute],
  shellSlots: { Layer: SetupLayer },
};
