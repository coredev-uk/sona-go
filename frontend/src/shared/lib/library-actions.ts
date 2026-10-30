import { toast } from "sonner";

import { confirm } from "./confirm";
import { callFailure } from "./errors";
import {
  addItemsToLibrary,
  changeItemState,
  type StateChange,
  type StateItem,
  type StateKind,
} from "./item-state";
import { run } from "./run";

/**
 * The library's item commands with their feedback (toasts, the removal
 * confirmation), shared by header controls and menu actions.
 */

const failure: Record<StateChange, string> = {
  favorite: "Couldn't favourite",
  unfavorite: "Couldn't undo favourite for",
  "suggest-less": "Couldn't suggest less of",
  "undo-suggest-less": "Couldn't undo suggest less for",
  remove: "Couldn't remove",
};

/** Changes an item's rating, with a toast when it fails. */
export function rateItem(kind: StateKind, item: StateItem, name: string, change: StateChange) {
  changeItemState(kind, item, change).catch((err) =>
    toast.error(`${failure[change]} “${name}”`, { description: callFailure(err).message }),
  );
}

/** Adds catalog items to the library, with a toast either way. */
export async function addToLibrary(
  kind: "songs" | "albums" | "playlists" | "music-videos",
  items: StateItem[],
  name: string,
): Promise<boolean> {
  const ok = await run(`Couldn't add “${name}” to your library`, addItemsToLibrary(kind, items));
  if (ok) toast(`Added “${name}” to your library`);
  return ok;
}

/** Kinds Sona removes from the library itself (playlists are deleted by the playlist editor). */
export type RemovableKind = "songs" | "albums" | "artists" | "music-videos";

const removeNote: Record<RemovableKind, string> = {
  songs: "It also leaves any playlists it's in.",
  albums: "Its songs leave your library and any playlists they're in.",
  artists: "Their albums and songs leave your library too.",
  "music-videos": "It also leaves any playlists it's in.",
};

/** Asks, then removes an item from the library; resolves whether it was removed. */
export async function removeFromLibrary(
  kind: RemovableKind,
  item: StateItem,
  name: string,
): Promise<boolean> {
  const ok = await confirm({
    title: `Remove “${name}” from your library?`,
    body: removeNote[kind],
    confirmLabel: "Remove",
    destructive: true,
  });
  if (!ok) return false;
  const done = await run(`Couldn't remove “${name}”`, changeItemState(kind, item, "remove"));
  if (done) toast(`Removed “${name}” from your library`);
  return done;
}
