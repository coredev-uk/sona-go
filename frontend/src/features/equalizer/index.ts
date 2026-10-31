import { t } from "@lingui/core/macro";
import { createRoute } from "@tanstack/react-router";
import { SlidersVertical } from "lucide-react";

import { currentCapabilities } from "@/shared/lib/capabilities";
import type { FeatureModule } from "@/shared/module";
import { childRoute } from "@/shared/router";

import { EqualizerPage } from "./EqualizerPage";

export { EqualizerButton } from "./EqualizerButton";

const equalizerRoute = createRoute({ ...childRoute("/equalizer"), component: EqualizerPage });

/**
 * The equaliser page (presets, ten bands and a preamp, previewed live and
 * saved in Go's settings), reached from the player bar, Settings →
 * Playback and Ctrl+Shift+E.
 */
export const equalizerModule: FeatureModule<typeof equalizerRoute> = {
  id: "equalizer",
  routes: [equalizerRoute],
  shortcuts: [
    {
      id: "equalizer.open",
      label: () => t`Open the equaliser`,
      group: "pages",
      keys: ["Ctrl+Shift+E"],
      keywords: () => t`eq bass treble sound`,
      icon: SlidersVertical,
      when: () => currentCapabilities()?.playback.equalizer ?? false,
      run: ({ navigate }) => void navigate({ to: "/equalizer" }),
    },
  ],
};
