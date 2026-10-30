import type { PlaylistFolder, PlaylistItem } from "@bindings/services";
import { create } from "zustand";

/** The playlist dialog that is open, if any (one at a time). */
export type PlaylistDialog =
  /** Pick a playlist to add `items` to; `label` names what is added. */
  | { kind: "add"; items: PlaylistItem[]; label: string }
  /** A new playlist, holding `items`, in folder `folderId` ("" top level). */
  | { kind: "create"; items: PlaylistItem[]; label: string; folderId: string }
  /** Name, description and visibility of a library playlist. */
  | {
      kind: "details";
      id: string;
      name: string;
      description: string;
      isPublic: boolean;
    }
  /** A new folder, or renaming `folder`. */
  | { kind: "folder"; folder?: PlaylistFolder }
  /** Move a library playlist to another folder. */
  | { kind: "move"; id: string; name: string; folderId: string };

interface DialogStore {
  open: PlaylistDialog | null;
  show: (d: PlaylistDialog) => void;
  close: () => void;
}

export const useDialogs = create<DialogStore>((set) => ({
  open: null,
  show: (open) => set({ open }),
  close: () => set({ open: null }),
}));

export const showDialog = (d: PlaylistDialog) => useDialogs.getState().show(d);
