import { PrivateService } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { HatGlasses } from "lucide-react";

import type { FeatureModule } from "@/shared/module";

import { applyPrivate, isForced, setPrivate, usePrivate } from "./state";

export { PrivateButton } from "./PrivateButton";
export { isForced, setPrivate, usePrivate } from "./state";

/**
 * Private listening: Go's private mode mirrored from private:changed (the
 * Settings row, the tray and `sona private` change it too), the player
 * bar's switch and a palette command. No default keys: it is a deliberate
 * switch, rarely flipped, and a stray key press would change what Apple
 * records; it can be bound in Settings → Shortcuts.
 */
export const privateModule: FeatureModule<never> = {
  id: "private",
  routes: [],
  shortcuts: [
    {
      id: "private.toggle",
      label: () =>
        usePrivate.getState().state?.on
          ? t`Turn off private listening`
          : t`Turn on private listening`,
      group: "playback",
      keys: [],
      keywords: () => t`private incognito history scrobble discord listening`,
      icon: HatGlasses,
      when: () => {
        const s = usePrivate.getState().state;
        return s !== null && !isForced(s);
      },
      run: () => {
        const s = usePrivate.getState().state;
        if (s) void setPrivate(!s.on);
      },
    },
  ],
  eventHandlers: {
    "private:changed": (data) => applyPrivate(data),
  },
  init: () => {
    void PrivateService.State().then(applyPrivate, () => {});
  },
};
