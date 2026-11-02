import { t } from "@lingui/core/macro";
import { SystemService } from "@bindings/services";
import { createRoute, redirect } from "@tanstack/react-router";
import { Settings } from "lucide-react";

import { openAccountDialog } from "@/shared/lib/account";
import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { integrationsStatusKey } from "./integrations";
import { loadAppearance } from "./queries";
import { scrobbleStatusKey } from "./scrobble";
import { namesAccountSection, settingsSearch } from "./search-params";
import { SettingsPage } from "./Settings";

export { bindAppearance, useAppearance, type Theme } from "./appearance";
export { loadAppearance, useSettings, useUpdateSettings } from "./queries";

const settingsRoute = createRoute({
  ...childRoute("/settings"),
  validateSearch: settingsSearch,
  // Settings → Account moved to the top bar's account menu: an old link
  // (or a saved last page) opens the account dialog over Settings.
  beforeLoad: ({ location }) => {
    if (!namesAccountSection(location.search)) return;
    openAccountDialog();
    throw redirect({ to: "/settings", replace: true });
  },
  component: SettingsPage,
});

/** Settings: the page and its sections, Go's settings, and the appearance mirror. */
export const settingsModule: FeatureModule<typeof settingsRoute> = {
  id: "settings",
  routes: [settingsRoute],
  nav: [
    {
      section: "footer",
      label: () => t`Settings`,
      icon: Settings,
      to: "/settings",
      order: 10,
      keys: ["Ctrl+,"],
    },
  ],
  eventHandlers: {
    "settings:changed": (data, { qc }) => {
      qc.setQueryData(keys.settings, data);
      loadAppearance(data);
    },
    "integrations:status": (data, { qc }) => qc.setQueryData(integrationsStatusKey, data),
    "scrobble:changed": (data, { qc }) => qc.setQueryData(scrobbleStatusKey, data),
  },
  init: ({ qc }) => {
    void qc
      .fetchQuery({ queryKey: keys.settings, queryFn: () => SystemService.Settings() })
      .then(loadAppearance, () => {});
  },
};
