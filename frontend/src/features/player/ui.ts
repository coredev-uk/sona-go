import type { QueueItem } from "@bindings/services";
import { create } from "zustand";

import { lyricsSongId, usePlayback } from "./playback";

/**
 * The panels the player can open over the page: the now-playing view, its
 * full-screen lyrics ("room": sized to be read across a room), and the
 * video view while a music video plays.
 */
export type Overlay = "none" | "now-playing" | "room" | "video";

/** What the now-playing view shows beside the cover: the lyric sheet or what plays next. */
export type Pane = "lyrics" | "queue";

interface UIStore {
  /** The queue beside the page (while the now-playing view is closed). */
  queueOpen: boolean;
  overlay: Overlay;
  /** The now-playing view's pane; kept while it is closed, so it reopens where it was. */
  pane: Pane;
  /** Now playing was reopened by leaving full-screen lyrics: focus goes back to its Full screen button. */
  fromRoom: boolean;
  toggleQueue: () => void;
  setQueueOpen: (open: boolean) => void;
  /** Opens the now-playing view, on `pane` when given, else on the pane it showed last. */
  openNowPlaying: (pane?: Pane) => void;
  /** Full-screen lyrics, over the whole window. */
  openRoom: () => void;
  /** The playing music video's picture, over the whole window. */
  openVideo: () => void;
  /** Steps back one level: full-screen lyrics → now playing → the page; the video view → the page. */
  closeOverlay: () => void;
  setPane: (pane: Pane) => void;
}

/** The player bar's queue toggle: the queue's Escape returns focus to it. */
export const QUEUE_TOGGLE_ID = "player-queue-toggle";
/** Now playing's Full screen button: leaving full-screen lyrics returns focus to it. */
export const ROOM_TOGGLE_ID = "player-room-toggle";

export const useUI = create<UIStore>((set) => ({
  queueOpen: false,
  overlay: "none",
  pane: "lyrics",
  fromRoom: false,
  toggleQueue: () => set((s) => ({ queueOpen: !s.queueOpen })),
  setQueueOpen: (queueOpen) => set({ queueOpen }),
  openNowPlaying: (pane) =>
    set((s) => ({ overlay: "now-playing", pane: pane ?? s.pane, fromRoom: false })),
  openRoom: () => set({ overlay: "room", pane: "lyrics" }),
  openVideo: () => set({ overlay: "video", fromRoom: false }),
  closeOverlay: () =>
    set((s) => ({
      overlay: s.overlay === "room" ? "now-playing" : "none",
      fromRoom: s.overlay === "room",
    })),
  setPane: (pane) => set({ pane }),
}));

/**
 * The pane the view shows for `pane` and the playing item: a station, a music
 * video or a song outside the catalogue has no lyrics, so it shows Up next.
 */
export function shownPane(pane: Pane, current: QueueItem | null): Pane {
  return lyricsSongId(current) && !current?.live ? pane : "queue";
}

/**
 * The queue is on screen: the now-playing view's Up next pane while the
 * view is open, else the queue beside the page.
 */
export function queueShown(
  s: Pick<UIStore, "overlay" | "pane" | "queueOpen">,
  current: QueueItem | null,
): boolean {
  if (s.overlay === "now-playing") return shownPane(s.pane, current) === "queue";
  return s.overlay === "none" && s.queueOpen;
}

/**
 * The player bar's Queue toggle and its shortcut: with the now-playing view
 * open it turns to Up next, or closes the view when Up next is showing (as
 * Lyrics does for the lyric sheet); otherwise it shows or hides the queue
 * beside the page.
 */
export function toggleQueueView(): void {
  const s = useUI.getState();
  if (s.overlay !== "now-playing") s.toggleQueue();
  else if (shownPane(s.pane, usePlayback.getState().current) === "queue") s.closeOverlay();
  else s.setPane("queue");
}
