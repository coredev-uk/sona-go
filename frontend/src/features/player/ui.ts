import { create } from "zustand";

/** The panels the player can open over or beside the page. */
export type Overlay = "none" | "now-playing";

interface UIStore {
  queueOpen: boolean;
  overlay: Overlay;
  /** Lyrics or the cover on the now-playing view. */
  showLyrics: boolean;
  toggleQueue: () => void;
  setQueueOpen: (open: boolean) => void;
  openNowPlaying: (lyrics: boolean) => void;
  closeOverlay: () => void;
  setShowLyrics: (on: boolean) => void;
}

export const useUI = create<UIStore>((set) => ({
  queueOpen: false,
  overlay: "none",
  showLyrics: true,
  toggleQueue: () => set((s) => ({ queueOpen: !s.queueOpen })),
  setQueueOpen: (queueOpen) => set({ queueOpen }),
  openNowPlaying: (showLyrics) => set({ overlay: "now-playing", showLyrics }),
  closeOverlay: () => set({ overlay: "none" }),
  setShowLyrics: (showLyrics) => set({ showLyrics }),
}));
