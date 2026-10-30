import { LibraryService, Source, type ItemState } from "@bindings/services";
import { Link2, Plus, Star, ThumbsDown, Trash2 } from "lucide-react";
import { toast } from "sonner";

import type {
  ActionDef,
  ActionTargetKind,
  ActionTargets,
  AnyActionDef,
  MenuData,
} from "@/shared/actions/registry";
import {
  addToLibrary,
  rateItem,
  removeFromLibrary,
  type RemovableKind,
} from "@/shared/lib/library-actions";
import { fetchItemState, itemRef, type StateKind } from "@/shared/lib/item-state";
import { run } from "@/shared/lib/run";

/** Copies an item's music.apple.com link (Go writes the clipboard). */
export async function copyLink(kind: StateKind | "stations", item: Parameters<typeof itemRef>[1]) {
  if (await run("Couldn't copy the link", LibraryService.CopyLink(itemRef(kind, item)))) {
    toast("Link copied");
  }
}

interface Options {
  /** Offered "Add to library" (catalog items). */
  add?: "songs" | "albums" | "playlists";
  /** Offered "Remove from library" (library items). */
  remove?: RemovableKind;
}

/**
 * The library's item actions for one target kind: add to or remove from
 * the library, Favourite, Suggest Less and Copy link. Their labels and
 * availability read the item's library state, loaded (batched) when the
 * menu opens.
 */
function itemActions<K extends Exclude<ActionTargetKind, "station">>(
  target: K,
  kind: StateKind,
  opts: Options,
): ActionDef<K>[] {
  const state: MenuData<K, ItemState | undefined> = {
    id: `library.state.${target}`,
    load: (t) => fetchItemState(kind, t),
  };
  const inLibrary = (t: ActionTargets[K], s: ItemState | undefined) =>
    s?.inLibrary ?? t.source === Source.SourceLibrary;
  const defs: ActionDef<K>[] = [
    {
      id: `library.favorite.${target}`,
      target,
      label: (_, ctx) => (ctx.data(state)?.rating === 1 ? "Undo favourite" : "Favourite"),
      icon: Star,
      filled: (_, ctx) => ctx.data(state)?.rating === 1,
      group: "rating",
      order: 10,
      data: [state],
      isEnabled: (_, ctx) => ctx.data(state) !== undefined,
      run: (t, ctx) =>
        rateItem(kind, t, t.name, ctx.data(state)?.rating === 1 ? "unfavorite" : "favorite"),
    },
    {
      id: `library.suggest-less.${target}`,
      target,
      label: (_, ctx) => (ctx.data(state)?.rating === -1 ? "Undo suggest less" : "Suggest less"),
      icon: ThumbsDown,
      filled: (_, ctx) => ctx.data(state)?.rating === -1,
      group: "rating",
      order: 20,
      data: [state],
      isEnabled: (_, ctx) => ctx.data(state) !== undefined,
      run: (t, ctx) =>
        rateItem(
          kind,
          t,
          t.name,
          ctx.data(state)?.rating === -1 ? "undo-suggest-less" : "suggest-less",
        ),
    },
    {
      id: `library.copy-link.${target}`,
      target,
      label: "Copy link",
      icon: Link2,
      group: "item",
      order: 90,
      isAvailable: (t) => t.source === Source.SourceCatalog || !!t.catalogId,
      run: (t) => void copyLink(kind, t),
    },
  ];
  const { add, remove } = opts;
  if (add) {
    defs.push({
      id: `library.add.${target}`,
      target,
      label: "Add to library",
      icon: Plus,
      group: "item",
      order: 30,
      data: [state],
      isAvailable: (t, ctx) => t.source === Source.SourceCatalog && !inLibrary(t, ctx.data(state)),
      isEnabled: (_, ctx) => ctx.data(state) !== undefined,
      run: (t) => void addToLibrary(add, [t], t.name),
    });
  }
  if (remove) {
    defs.push({
      id: `library.remove.${target}`,
      target,
      label: "Remove from library",
      icon: Trash2,
      group: "item",
      order: 35,
      data: [state],
      isAvailable: (t, ctx) => inLibrary(t, ctx.data(state)),
      run: (t) => void removeFromLibrary(remove, t, t.name),
    });
  }
  return defs;
}

/** Item actions for songs, albums, artists and playlists (library playlists are edited by the playlist module). */
export const libraryActions: AnyActionDef[] = [
  ...itemActions("song", "songs", { add: "songs", remove: "songs" }),
  ...itemActions("album", "albums", { add: "albums", remove: "albums" }),
  ...itemActions("artist", "artists", { remove: "artists" }),
  ...itemActions("playlist", "playlists", { add: "playlists" }),
  {
    id: "library.copy-link.station",
    target: "station",
    label: "Copy link",
    icon: Link2,
    group: "item",
    order: 90,
    run: (st) => void copyLink("stations", { id: st.id, source: Source.SourceCatalog }),
  },
];
