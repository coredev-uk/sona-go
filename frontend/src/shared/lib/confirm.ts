import { create } from "zustand";

export interface ConfirmOptions {
  /** A question naming the item: "Delete “OK Computer” from your library?" */
  title: string;
  /** One sentence on what else happens. */
  body: string;
  /** The verb on the confirming button: "Delete". */
  confirmLabel: string;
  /** Styles the confirming button as destructive. */
  destructive?: boolean;
}

interface ConfirmStore {
  open: (ConfirmOptions & { resolve: (ok: boolean) => void }) | null;
  ask: (o: ConfirmOptions) => Promise<boolean>;
  settle: (ok: boolean) => void;
}

/** The pending confirmation, rendered by `ConfirmHost` (shared/components/ConfirmDialog). */
export const useConfirm = create<ConfirmStore>((set, get) => ({
  open: null,
  ask: (o) =>
    new Promise<boolean>((resolve) => {
      get().open?.resolve(false);
      set({ open: { ...o, resolve } });
    }),
  settle: (ok) => {
    get().open?.resolve(ok);
    set({ open: null });
  },
}));

/**
 * Asks the user to confirm an action that cannot be undone from Sona.
 * Resolves true when they confirm, false when they cancel or dismiss.
 * Needs a mounted `ConfirmHost` (the library module mounts one).
 */
export function confirm(o: ConfirmOptions): Promise<boolean> {
  return useConfirm.getState().ask(o);
}
