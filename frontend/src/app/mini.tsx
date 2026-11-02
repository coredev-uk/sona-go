import type { Settings } from "@bindings/config";
import { AuthService, PlaybackService, SystemService } from "@bindings/services";
import { Events } from "@wailsio/runtime";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "@/shared/styles/index.css";

import { MiniPlayer } from "@/features/miniplayer";
import { usePlayback } from "@/features/player";
import { bindAppearance, loadAppearance } from "@/features/settings";
import {
  followAccountLanguage,
  followLanguageSetting,
  initI18n,
  startTranslations,
} from "@/shared/i18n";
import { wrapTabFocus } from "@/shared/lib/focus-wrap";
import { shareEventDispatch } from "@/shared/lib/go-events";

import { AppProviders } from "./providers";

/**
 * Entry of the mini player window (miniplayer.html). The window has its
 * own web view, so it keeps its own copy of the playback mirror from the
 * same Go events and snapshot the main window uses, and follows the theme,
 * the Language setting and the account's language.
 */
shareEventDispatch();
initI18n();
startTranslations();
bindAppearance();
wrapTabFocus();
const loadSettings = (s: Settings) => {
  loadAppearance(s);
  followLanguageSetting(s.language);
};
Events.On("playback:state", ({ data }) => usePlayback.getState().apply(data));
Events.On("settings:changed", ({ data }) => loadSettings(data));
Events.On("auth:changed", ({ data }) => followAccountLanguage(data.language));
void PlaybackService.State().then(
  (s) => usePlayback.getState().apply(s),
  () => {},
);
void SystemService.Settings().then(loadSettings, () => {});
void AuthService.Status().then(
  (s) => followAccountLanguage(s.language),
  () => {},
);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AppProviders>
      <MiniPlayer />
    </AppProviders>
  </StrictMode>,
);
