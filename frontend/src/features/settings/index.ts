import { SystemService } from "@bindings/services";
import { createRoute } from "@tanstack/react-router";
import { Settings } from "lucide-react";

import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { loadAppearance } from "./queries";
import { settingsSearch } from "./search-params";
import { SettingsPage } from "./Settings";

export { bindAppearance, useAppearance, type Theme } from "./appearance";

const settingsRoute = createRoute({
  ...childRoute("/settings"),
  validateSearch: settingsSearch,
  component: SettingsPage,
});

/** Settings: the page and its sections, Go's settings, and the appearance mirror. */
export const settingsModule: FeatureModule<typeof settingsRoute> = {
  id: "settings",
  routes: [settingsRoute],
  nav: [{ section: "footer", label: "Settings", icon: Settings, to: "/settings", order: 10 }],
  eventHandlers: {
    "settings:changed": (data, { qc }) => {
      qc.setQueryData(keys.settings, data);
      loadAppearance(data);
    },
  },
  init: ({ qc }) => {
    void qc
      .fetchQuery({ queryKey: keys.settings, queryFn: () => SystemService.Settings() })
      .then(loadAppearance, () => {});
  },
};
