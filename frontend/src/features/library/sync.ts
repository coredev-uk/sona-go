import { create } from "zustand";

import type { SyncDone, SyncProgress } from "@bindings/services";

interface SyncStore {
  running: boolean;
  /** The domain being synced and how far it is. */
  current: SyncProgress | null;
  last: SyncDone | null;
  progress: (p: SyncProgress) => void;
  done: (d: SyncDone) => void;
  /** Seeds the indicator from LibraryService.SyncStatus at start-up. */
  seed: (running: boolean) => void;
}

/** Library sync progress from sync:progress / sync:done, for the sidebar indicator. */
export const useSync = create<SyncStore>((set) => ({
  running: false,
  current: null,
  last: null,
  progress: (p) => set({ running: true, current: p }),
  done: (d) => set({ running: false, current: null, last: d }),
  seed: (running) => set((s) => (s.current ? s : { running })),
}));
