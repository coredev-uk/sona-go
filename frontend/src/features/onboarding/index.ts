import { createRoute } from "@tanstack/react-router";

import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { SetupPage } from "./SetupPage";
import { setupSearch } from "./steps";

const welcomeRoute = createRoute({
  ...childRoute("/welcome"),
  validateSearch: setupSearch,
  component: SetupPage,
});

/**
 * The first-run setup: its own page (/welcome) with no sidebar link. Go
 * opens the main window on it on a first start (services.StartRoute);
 * Settings → General links to it.
 */
export const onboardingModule: FeatureModule<typeof welcomeRoute> = {
  id: "onboarding",
  routes: [welcomeRoute],
};
