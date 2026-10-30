import { create } from "zustand";

/** Which of the command surfaces is open (one at a time). */
export type CommandSurface = "none" | "palette" | "help";

interface CommandsStore {
  open: CommandSurface;
  /** Opens a surface (remembering where focus was) or closes with "none". */
  show: (surface: CommandSurface) => void;
  /** Puts focus back where it was before the first surface opened. */
  restoreFocus: () => void;
}

/** The element focused when a surface opened from none. */
let returnTo: HTMLElement | null = null;

export const useCommands = create<CommandsStore>((set, get) => ({
  open: "none",
  show: (open) => {
    if (get().open === "none" && open !== "none") {
      returnTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    }
    set({ open });
  },
  restoreFocus: () => {
    if (returnTo?.isConnected) returnTo.focus({ preventScroll: true });
    returnTo = null;
  },
}));
