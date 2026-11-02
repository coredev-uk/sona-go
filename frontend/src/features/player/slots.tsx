import { useMemo, type ReactNode } from "react";

import { NowPlayingContext } from "@/shared/lib/now-playing";
import { useReturnFocus } from "@/shared/lib/useReturnFocus";

import { NowPlayingView } from "./NowPlayingView";
import { RoomLyrics } from "./RoomLyrics";
import { usePlayback } from "./playback";
import { QueuePanel } from "./QueuePanel";
import { useUI } from "./ui";
import { VideoView } from "./VideoView";

/**
 * Gives shared lists (TrackList) the playing track, so they can mark it,
 * and whether the player's overlay covers them.
 */
export function NowPlayingProvider({ children }: { children: ReactNode }) {
  const current = usePlayback((s) => s.current);
  const playing = usePlayback((s) => s.state === "playing" || s.state === "loading");
  const covered = useUI((s) => s.overlay !== "none");
  const value = useMemo(() => ({ current, playing, covered }), [current, playing, covered]);
  return <NowPlayingContext.Provider value={value}>{children}</NowPlayingContext.Provider>;
}

/** The queue beside the page, while open. Closing it returns focus to its opener. */
export function QueueAside() {
  const queueOpen = useUI((s) => s.queueOpen);
  useReturnFocus(queueOpen);
  return queueOpen ? <QueuePanel /> : null;
}

/**
 * The full now-playing view over the page column, full-screen lyrics, or
 * the video view, while open. Stepping between them keeps focus inside;
 * closing returns it to whatever opened the first.
 */
export function NowPlayingOverlay() {
  const overlay = useUI((s) => s.overlay);
  useReturnFocus(overlay !== "none");
  if (overlay === "room") return <RoomLyrics />;
  if (overlay === "video") return <VideoView />;
  return overlay === "now-playing" ? <NowPlayingView /> : null;
}
