import { create } from "zustand";

/**
 * The panels the player can open over the page: the now-playing view, its
 * full-screen lyrics ("room": sized to be read across a room), and the
 * video view while a music video plays.
 */
export type Overlay = "none" | "now-playing" | "room" | "video";

interface UIStore {
  queueOpen: boolean;
  overlay: Overlay;
  /** Lyrics or the cover on the now-playing view. */
  showLyrics: boolean;
  /** Now playing was reopened by leaving full-screen lyrics: focus goes back to its Full screen button. */
  fromRoom: boolean;
  toggleQueue: () => void;
  setQueueOpen: (open: boolean) => void;
  openNowPlaying: (lyrics: boolean) => void;
  /** Full-screen lyrics, over the whole window. */
  openRoom: () => void;
  /** The playing music video's picture, over the whole window. */
  openVideo: () => void;
  /** Steps back one level: full-screen lyrics → now playing → the page; the video view → the page. */
  closeOverlay: () => void;
  setShowLyrics: (on: boolean) => void;
}

/** The player bar's queue toggle: the queue's Escape returns focus to it. */
export const QUEUE_TOGGLE_ID = "player-queue-toggle";
/** Now playing's Full screen button: leaving full-screen lyrics returns focus to it. */
export const ROOM_TOGGLE_ID = "player-room-toggle";

export const useUI = create<UIStore>((set) => ({
  queueOpen: false,
  overlay: "none",
  showLyrics: true,
  fromRoom: false,
  toggleQueue: () => set((s) => ({ queueOpen: !s.queueOpen })),
  setQueueOpen: (queueOpen) => set({ queueOpen }),
  openNowPlaying: (showLyrics) => set({ overlay: "now-playing", showLyrics, fromRoom: false }),
  openRoom: () => set({ overlay: "room", showLyrics: true }),
  openVideo: () => set({ overlay: "video", fromRoom: false }),
  closeOverlay: () =>
    set((s) => ({
      overlay: s.overlay === "room" ? "now-playing" : "none",
      fromRoom: s.overlay === "room",
    })),
  setShowLyrics: (showLyrics) => set({ showLyrics }),
}));
