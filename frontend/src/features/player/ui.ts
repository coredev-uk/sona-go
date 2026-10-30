import { create } from "zustand";

/**
 * The panels the player can open over the page: the now-playing view, and
 * its full-screen lyrics ("room": sized to be read across a room).
 */
export type Overlay = "none" | "now-playing" | "room";

interface UIStore {
  queueOpen: boolean;
  overlay: Overlay;
  /** Lyrics or the cover on the now-playing view. */
  showLyrics: boolean;
  toggleQueue: () => void;
  setQueueOpen: (open: boolean) => void;
  openNowPlaying: (lyrics: boolean) => void;
  /** Full-screen lyrics, over the whole window. */
  openRoom: () => void;
  /** Steps back one level: full-screen lyrics → now playing → the page. */
  closeOverlay: () => void;
  setShowLyrics: (on: boolean) => void;
}

/** The player bar's queue toggle: the queue's Escape returns focus to it. */
export const QUEUE_TOGGLE_ID = "player-queue-toggle";

export const useUI = create<UIStore>((set) => ({
  queueOpen: false,
  overlay: "none",
  showLyrics: true,
  toggleQueue: () => set((s) => ({ queueOpen: !s.queueOpen })),
  setQueueOpen: (queueOpen) => set({ queueOpen }),
  openNowPlaying: (showLyrics) => set({ overlay: "now-playing", showLyrics }),
  openRoom: () => set({ overlay: "room", showLyrics: true }),
  closeOverlay: () => set((s) => ({ overlay: s.overlay === "room" ? "now-playing" : "none" })),
  setShowLyrics: (showLyrics) => set({ showLyrics }),
}));
