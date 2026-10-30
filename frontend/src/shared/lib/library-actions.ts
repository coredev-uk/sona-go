import { t } from "@lingui/core/macro";
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

/** The failure toast for a rating change of `subject` (“Airbag”, "12 songs", "2 of 12 songs"). */
function failure(change: StateChange, subject: string): string {
  switch (change) {
    case "favorite":
      return t`Couldn't favourite ${subject}`;
    case "unfavorite":
      return t`Couldn't undo favourite for ${subject}`;
    case "suggest-less":
      return t`Couldn't suggest less of ${subject}`;
    case "undo-suggest-less":
      return t`Couldn't undo suggest less for ${subject}`;
    case "remove":
      return t`Couldn't remove ${subject}`;
  }
}

/** Changes an item's rating, with a toast when it fails. */
export function rateItem(kind: StateKind, item: StateItem, name: string, change: StateChange) {
  changeItemState(kind, item, change).catch((err) =>
    toast.error(failure(change, t`“${name}”`), { description: callFailure(err).message }),
  );
}

/** Adds catalog items to the library, with a toast either way. */
export async function addToLibrary(
  kind: "songs" | "albums" | "playlists" | "music-videos",
  items: StateItem[],
  name: string,
): Promise<boolean> {
  const ok = await run(t`Couldn't add “${name}” to your library`, addItemsToLibrary(kind, items));
  if (ok) toast(t`Added “${name}” to your library`);
  return ok;
}

/** Kinds Sona removes from the library itself (playlists are deleted by the playlist editor). */
export type RemovableKind = "songs" | "albums" | "artists" | "music-videos";

function removeNote(kind: RemovableKind): string {
  switch (kind) {
    case "songs":
    case "music-videos":
      return t`It also leaves any playlists it's in.`;
    case "albums":
      return t`Its songs leave your library and any playlists they're in.`;
    case "artists":
      return t`Their albums and songs leave your library too.`;
  }
}

/** Asks, then removes an item from the library; resolves whether it was removed. */
export async function removeFromLibrary(
  kind: RemovableKind,
  item: StateItem,
  name: string,
): Promise<boolean> {
  const ok = await confirm({
    title: t`Remove “${name}” from your library?`,
    body: removeNote(kind),
    confirmLabel: t`Remove`,
    destructive: true,
  });
  if (!ok) return false;
  const done = await run(t`Couldn't remove “${name}”`, changeItemState(kind, item, "remove"));
  if (done) toast(t`Removed “${name}” from your library`);
  return done;
}

/** One item of a bulk library command, with its state kind. */
export interface KindedItem {
  kind: StateKind;
  item: StateItem;
}

/**
 * Changes the rating of several items at once (each change shows at once);
 * one toast names how many failed.
 */
export async function rateItems(items: readonly KindedItem[], change: StateChange, what: string) {
  const results = await Promise.allSettled(
    items.map((i) => changeItemState(i.kind, i.item, change)),
  );
  const failed = results.filter((r) => r.status === "rejected");
  if (failed.length) {
    const count = failed.length;
    toast.error(failure(change, count === items.length ? what : t`${count} of ${what}`), {
      description: callFailure((failed[0] as PromiseRejectedResult).reason).message,
    });
  }
}

/** Adds catalog items of several kinds to the library, one call per kind, with one toast. */
export async function addManyToLibrary(items: readonly KindedItem[], what: string) {
  const byKind = new Map<StateKind, StateItem[]>();
  for (const i of items) byKind.set(i.kind, [...(byKind.get(i.kind) ?? []), i.item]);
  const calls = [...byKind].map(([kind, list]) =>
    addItemsToLibrary(kind as "songs" | "albums" | "playlists" | "music-videos", list),
  );
  const ok = await run(t`Couldn't add ${what} to your library`, Promise.all(calls));
  if (ok) toast(t`Added ${what} to your library`);
}

/** Asks once, then removes several items from the library. */
export async function removeManyFromLibrary(
  items: readonly (KindedItem & { kind: RemovableKind })[],
  what: string,
): Promise<boolean> {
  const ok = await confirm({
    title: t`Remove ${what} from your library?`,
    body: t`They also leave any playlists they're in.`,
    confirmLabel: t`Remove`,
    destructive: true,
  });
  if (!ok) return false;
  const done = await run(
    t`Couldn't remove ${what}`,
    Promise.all(items.map((i) => changeItemState(i.kind, i.item, "remove"))),
  );
  if (done) toast(t`Removed ${what} from your library`);
  return done;
}
