import { DesktopService } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { PictureInPicture2 } from "lucide-react";

import { run } from "@/shared/lib/run";
import type { FeatureModule } from "@/shared/module";

export { MiniPlayer } from "./MiniPlayer";

/** Opens the mini player in place of the main window, or returns from it. */
export function toggleMiniPlayer() {
  void run(t`Couldn't open the mini player`, DesktopService.ToggleMiniPlayer());
}

/**
 * The mini player: a second window (miniplayer.html, rendered by
 * app/mini.tsx) with the cover, song, lyric line, transport and seek. In
 * the main window this module only contributes its shortcut; the player
 * bar and the tray open it too.
 */
export const miniplayerModule: FeatureModule<never> = {
  id: "miniplayer",
  routes: [],
  shortcuts: [
    {
      id: "miniplayer.toggle",
      label: () => t`Show the mini player`,
      group: "view",
      keys: ["Ctrl+Shift+M"],
      keywords: () => t`compact small window`,
      icon: PictureInPicture2,
      run: toggleMiniPlayer,
    },
  ],
};
