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
export { useLanguageOptions, useSetLanguage } from "./language-queries";
export { LanguageSelect } from "./LanguageRow";
export { loadAppearance, useSettings, useUpdateSettings } from "./queries";
export { useIntegrationsStatus } from "./integrations";
export { PROVIDER_NAMES, scrobbleStatusKey, useConnectLastFM, useScrobbleStatus } from "./scrobble";
// The scrobbler connection flows the first-run setup's Connections step shares.
export { LastFMWaiting, ListenBrainzForm } from "./Connections";

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

/** The lyrics settings last seen, to refetch lyrics when they change. */
let lyricsSettings = "";

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
      // Another source, order or language can pick other lyrics.
      const lyrics = JSON.stringify(data.lyrics);
      if (lyricsSettings && lyrics !== lyricsSettings) {
        void qc.invalidateQueries({ queryKey: keys.lyricsAll });
      }
      lyricsSettings = lyrics;
    },
    "integrations:status": (data, { qc }) => qc.setQueryData(integrationsStatusKey, data),
    "scrobble:changed": (data, { qc }) => qc.setQueryData(scrobbleStatusKey, data),
    "translations:languages": (_data, { qc }) =>
      void qc.invalidateQueries({ queryKey: keys.translationsLanguages }),
  },
  init: ({ qc }) => {
    void qc.fetchQuery({ queryKey: keys.settings, queryFn: () => SystemService.Settings() }).then(
      (s) => {
        lyricsSettings = JSON.stringify(s.lyrics);
        loadAppearance(s);
      },
      () => {},
    );
  },
};
