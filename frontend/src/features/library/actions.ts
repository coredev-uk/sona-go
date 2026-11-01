import { LibraryService, Source, type ItemState } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { Link2, Plus, Star, ThumbsDown, Trash2 } from "lucide-react";
import { toast } from "sonner";

import {
  describeItems,
  type ActionDef,
  type ActionItem,
  type ActionTargetKind,
  type ActionTargets,
  type BulkActionDef,
  type BulkMenuData,
  type ItemActionDef,
  type MenuData,
} from "@/shared/actions/registry";
import {
  addManyToLibrary,
  addToLibrary,
  rateItem,
  rateItems,
  removeFromLibrary,
  removeManyFromLibrary,
  type KindedItem,
  type RemovableKind,
} from "@/shared/lib/library-actions";
import { fetchItemState, itemRef, type StateKind } from "@/shared/lib/item-state";
import { isUpcoming } from "@/shared/lib/release";
import { run } from "@/shared/lib/run";

/** Copies an item's music.apple.com link (Go writes the clipboard). */
export async function copyLink(kind: StateKind | "stations", item: Parameters<typeof itemRef>[1]) {
  if (await run(t`Couldn't copy the link`, LibraryService.CopyLink(itemRef(kind, item)))) {
    toast(t`Link copied`);
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
      label: (_, ctx) => (ctx.data(state)?.rating === 1 ? t`Undo favourite` : t`Favourite`),
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
      label: (_, ctx) => (ctx.data(state)?.rating === -1 ? t`Undo suggest less` : t`Suggest less`),
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
      label: () => t`Copy link`,
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
      // An album not out yet is pre-added: the same call (Apple's web player's too).
      label: (item) =>
        target === "album" && isUpcoming(item as ActionTargets["album"])
          ? t`Pre-add to library`
          : t`Add to library`,
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
      label: () => t`Remove from library`,
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
export const libraryActions: ItemActionDef[] = [
  ...itemActions("song", "songs", { add: "songs", remove: "songs" }),
  ...itemActions("album", "albums", { add: "albums", remove: "albums" }),
  ...itemActions("artist", "artists", { remove: "artists" }),
  ...itemActions("playlist", "playlists", { add: "playlists" }),
  {
    id: "library.copy-link.station",
    target: "station",
    label: () => t`Copy link`,
    icon: Link2,
    group: "item",
    order: 90,
    run: (st) => void copyLink("stations", { id: st.id, source: Source.SourceCatalog }),
  },
  ...bulkActions(),
];

const stateKinds: Record<Exclude<ActionTargetKind, "station">, StateKind> = {
  song: "songs",
  album: "albums",
  artist: "artists",
  playlist: "playlists",
};

/** The selection with each item's state kind (stations have none). */
function kinded(items: readonly ActionItem[]): KindedItem[] {
  return items.flatMap((i) =>
    i.kind === "station" ? [] : [{ kind: stateKinds[i.kind], item: i.target }],
  );
}

/**
 * The `targets[]` forms of the library actions: Favourite, add to and
 * remove from the library for a selection. Their labels and availability
 * read every selected item's state (batched into one Go call when the menu
 * opens).
 */
function bulkActions(): BulkActionDef[] {
  const states: BulkMenuData<(ItemState | undefined)[]> = {
    id: "library.state.many",
    load: (items) => Promise.all(kinded(items).map((k) => fetchItemState(k.kind, k.item))),
  };
  type States = (ItemState | undefined)[] | undefined;
  const inLibrary = (items: readonly ActionItem[], s: States) =>
    kinded(items).map((k, i) => s?.[i]?.inLibrary ?? k.item.source === Source.SourceLibrary);
  /** Catalogue items not (yet) in the library. */
  const notInLibrary = (items: readonly ActionItem[], s: States) =>
    kinded(items).filter((k, i) => k.item.source === Source.SourceCatalog && !s?.[i]?.inLibrary);
  const allFavourite = (s: States) => !!s?.length && s.every((x) => x?.rating === 1);
  const music: ActionTargetKind[] = ["song", "album", "artist", "playlist"];
  return [
    {
      id: "library.favorite.many",
      target: "selection",
      kinds: music,
      label: (_, ctx) => (allFavourite(ctx.data(states)) ? t`Undo favourite` : t`Favourite`),
      icon: Star,
      filled: (_, ctx) => allFavourite(ctx.data(states)),
      group: "rating",
      order: 10,
      data: [states],
      isEnabled: (_, ctx) => ctx.data(states) !== undefined,
      run: (items, ctx) => {
        const undo = allFavourite(ctx.data(states));
        // Favouriting skips items that already are; undoing applies to all.
        const s = ctx.data(states);
        const targets = kinded(items).filter((_, i) => undo || s?.[i]?.rating !== 1);
        void rateItems(targets, undo ? "unfavorite" : "favorite", describeItems(items));
      },
    },
    {
      id: "library.add.many",
      target: "selection",
      kinds: ["song", "album", "playlist"],
      label: () => t`Add to library`,
      icon: Plus,
      group: "item",
      order: 30,
      data: [states],
      isAvailable: (items, ctx) => notInLibrary(items, ctx.data(states)).length > 0,
      isEnabled: (_, ctx) => ctx.data(states) !== undefined,
      run: (items, ctx) =>
        void addManyToLibrary(notInLibrary(items, ctx.data(states)), describeItems(items)),
    },
    {
      id: "library.remove.many",
      target: "selection",
      kinds: ["song", "album", "artist"],
      label: () => t`Remove from library`,
      icon: Trash2,
      group: "item",
      order: 35,
      data: [states],
      isAvailable: (items, ctx) => inLibrary(items, ctx.data(states)).every(Boolean),
      run: (items) =>
        void removeManyFromLibrary(
          kinded(items) as (KindedItem & { kind: RemovableKind })[],
          describeItems(items),
        ),
    },
  ];
}
