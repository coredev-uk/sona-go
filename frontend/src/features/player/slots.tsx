import { useMemo, type ReactNode } from "react";

import { NowPlayingContext } from "@/shared/lib/now-playing";

import { NowPlayingView } from "./NowPlayingView";
import { usePlayback } from "./playback";
import { QueuePanel } from "./QueuePanel";
import { useUI } from "./ui";

/** Gives shared lists (TrackList) the playing track, so they can mark it. */
export function NowPlayingProvider({ children }: { children: ReactNode }) {
  const current = usePlayback((s) => s.current);
  const playing = usePlayback((s) => s.state === "playing" || s.state === "loading");
  const value = useMemo(() => ({ current, playing }), [current, playing]);
  return <NowPlayingContext.Provider value={value}>{children}</NowPlayingContext.Provider>;
}

/** The queue beside the page, while open. */
export function QueueAside() {
  const queueOpen = useUI((s) => s.queueOpen);
  return queueOpen ? <QueuePanel /> : null;
}

/** The full now-playing view over the page column, while open. */
export function NowPlayingOverlay() {
  const overlay = useUI((s) => s.overlay);
  return overlay === "now-playing" ? <NowPlayingView /> : null;
}
