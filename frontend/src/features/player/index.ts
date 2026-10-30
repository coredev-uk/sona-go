import { PlaybackService } from "@bindings/services";

import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";

import { playerActions } from "./actions";
import { NowPlayingBar } from "./NowPlayingBar";
import { usePlayback } from "./playback";
import { playerShortcuts } from "./shortcuts";
import { NowPlayingOverlay, NowPlayingProvider, QueueAside } from "./slots";
import { useUI } from "./ui";

export { playSongs, queueItems, shuffleSongs, type PlaySource } from "./commands";
/** Starting a new queue (a station, the library): asks or offers Undo over a long user queue. */
export { replaceQueue } from "./replace";
/** The playback mirror, lyrics and track announcements, for a second window (the mini player). */
export { livePosition, useLivePosition, usePlayback } from "./playback";
export { activeLine } from "./lyrics";
export { useLyrics } from "./queries";
export { TrackAnnouncer } from "./TrackAnnouncer";

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
