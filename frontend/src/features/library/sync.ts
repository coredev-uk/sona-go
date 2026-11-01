import { create } from "zustand";

import type { SyncDone, SyncProgress } from "@bindings/services";

interface SyncStore {
  running: boolean;
  /** The domain being synced and how far it is. */
  current: SyncProgress | null;
  last: SyncDone | null;
  /** How many sync:progress and sync:done events have arrived. */
  events: number;
  progress: (p: SyncProgress) => void;
  done: (d: SyncDone) => void;
  /**
   * Applies a LibraryService.SyncStatus answer requested when `events` was
   * `at`. Events and call results reach the page by separate paths, so an
   * answer can arrive after events Go sent later than it; once any event
   * has arrived since the request, the events carry the newer state and the
   * answer is dropped.
   */
  snapshot: (running: boolean, at: number) => void;
}

/** Library sync progress from sync:progress / sync:done, for the sidebar indicator. */
export const useSync = create<SyncStore>((set) => ({
  running: false,
  current: null,
  last: null,
  events: 0,
  progress: (p) => set((s) => ({ running: true, current: p, events: s.events + 1 })),
  done: (d) => set((s) => ({ running: false, current: null, last: d, events: s.events + 1 })),
  snapshot: (running, at) =>
    set((s) => {
      if (s.events !== at || s.running === running) return s;
      return running ? { running } : { running, current: null };
    }),
}));
