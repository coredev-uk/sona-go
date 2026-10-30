import { PlaybackService } from "@bindings/services";

import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";

import { playerActions } from "./actions";
import { NowPlayingBar } from "./NowPlayingBar";
import { usePlayback } from "./playback";
import { playerShortcuts } from "./shortcuts";
import { NowPlayingOverlay, NowPlayingProvider, QueueAside } from "./slots";
import { useUI } from "./ui";

export { addToQueue, playNext, playSongs, shuffleSongs } from "./commands";

/**
 * The player: Go's playback mirror, the now-playing bar, queue and view,
 * media-key shortcuts and the queue actions on song rows.
 */
export const playerModule: FeatureModule<never> = {
  id: "player",
  routes: [],
  shortcuts: playerShortcuts,
  actions: playerActions,
  eventHandlers: {
    "playback:state": (data, { invalidate }) => {
      const prev = usePlayback.getState().current;
      usePlayback.getState().apply(data);
      // A new track makes Apple's recently played list stale.
      if (data.current && data.current.id !== prev?.id) invalidate([keys.libraryRecent]);
    },
    "playback:queue": (data) => usePlayback.getState().applyQueue(data),
  },
  init: () => {
    void PlaybackService.State().then(
      (s) => usePlayback.getState().apply(s),
      () => {},
    );
    void PlaybackService.Queue().then(
      (q) => usePlayback.getState().applyQueue(q),
      () => {},
    );
  },
  shellSlots: {
    Provider: NowPlayingProvider,
    Aside: QueueAside,
    overlay: {
      Component: NowPlayingOverlay,
      useOpen: () => useUI((s) => s.overlay !== "none"),
    },
    Bottom: NowPlayingBar,
  },
};
