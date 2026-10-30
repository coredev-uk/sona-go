import { LibraryService, Source, type ItemRef, type ItemState } from "@bindings/services";
import { useQuery, type QueryClient, type QueryKey } from "@tanstack/react-query";

import { keys } from "./keys";

/**
 * Library state and ratings for any item a page shows: whether it is in
 * the library (and its library ID, which removal needs), and whether it is
 * a Favourite (1) or Suggest Less (-1). States load in batches: every
 * request made within one frame becomes one `LibraryService.States` call,
 * which Go answers from a short cache or one Apple lookup per batch.
 */

/** Apple's catalog type for an item. */
export type StateKind = "songs" | "albums" | "artists" | "playlists" | "music-videos";

/** What the item DTOs (Song, Album, Artist, Playlist, MusicVideo) share. */
export interface StateItem {
  id: string;
  source: Source;
  catalogId?: string;
  libraryId?: string;
}

export function itemRef(kind: StateKind | "stations", item: StateItem): ItemRef {
  return {
    kind,
    id: item.id,
    source: item.source,
    catalogId: item.catalogId ?? "",
    libraryId: item.libraryId ?? "",
  };
}

interface Pending {
  ref: ItemRef;
  resolve: (s: ItemState) => void;
  reject: (e: unknown) => void;
}

let queue: Pending[] = [];
let scheduled = false;

async function flush() {
  scheduled = false;
  const batch = queue;
  queue = [];
  try {
    const states = await LibraryService.States(batch.map((p) => p.ref));
    batch.forEach((p, i) => p.resolve(states[i]));
  } catch (err) {
    batch.forEach((p) => p.reject(err));
  }
}

/** Loads one item's state, batched with every other load in the same frame. */
export function loadItemState(kind: StateKind, item: StateItem): Promise<ItemState> {
  return new Promise((resolve, reject) => {
    queue.push({ ref: itemRef(kind, item), resolve, reject });
    if (!scheduled) {
      scheduled = true;
      setTimeout(() => void flush(), 16);
    }
  });
}

/** States are refreshed after a minute; Go caches Apple's answers for two. */
const STALE_MS = 60_000;

const stateQuery = (kind: StateKind, item: StateItem) => ({
  queryKey: keys.itemState(kind, item.id),
  queryFn: () => loadItemState(kind, item),
  staleTime: STALE_MS,
});

/** An item's library state and rating; `undefined` while it loads. */
export function useItemState(kind: StateKind, item: StateItem | undefined) {
  return useQuery({
    ...stateQuery(kind, item ?? { id: "", source: Source.SourceCatalog }),
    enabled: !!item?.id,
  });
}

let client: QueryClient | null = null;

/**
 * Gives the item-state helpers the app's query client, so changes made
 * outside React (menu actions) update every view of the item. The library
 * module binds it at start-up.
 */
export function bindItemStates(qc: QueryClient) {
  client = qc;
}

function qc(): QueryClient {
  if (!client) throw new Error("item states: no query client bound");
  return client;
}

/** Reads an item's state through the cache (menus); `undefined` if it cannot load. */
export function fetchItemState(kind: StateKind, item: StateItem): Promise<ItemState | undefined> {
  return qc()
    .fetchQuery(stateQuery(kind, item))
    .catch(() => undefined);
}

/** Library lists to refresh when an item joins or leaves the library. */
const membershipLists: QueryKey[] = [
  keys.libraryCounts,
  ["library", "songs"],
  ["library", "albums"],
  ["library", "artists"],
  ["library", "album"],
  ["library", "artist"],
  ["library", "recently-added"],
  ["library", "favorite-songs"],
];

export type StateChange =
  "favorite" | "unfavorite" | "suggest-less" | "undo-suggest-less" | "remove";

/**
 * Changes an item's rating or removes it from the library: the new state
 * shows at once, Go's answer replaces it, and a failure restores the old
 * state and rejects. Lists that the change affects are refreshed.
 */
export async function changeItemState(
  kind: StateKind,
  item: StateItem,
  change: StateChange,
): Promise<ItemState> {
  const key = keys.itemState(kind, item.id);
  const prev = qc().getQueryData<ItemState>(key);
  const base: ItemState = prev ?? {
    kind,
    id: item.id,
    inLibrary: item.source === Source.SourceLibrary,
    libraryId: item.libraryId ?? "",
    rating: 0,
  };
  const optimistic: ItemState = { ...base };
  if (change === "favorite") optimistic.rating = 1;
  else if (change === "suggest-less") optimistic.rating = -1;
  else if (change === "remove") Object.assign(optimistic, { inLibrary: false, libraryId: "" });
  else optimistic.rating = 0;
  await qc().cancelQueries({ queryKey: key });
  qc().setQueryData(key, optimistic);

  const ref = itemRef(kind, item);
  let call: Promise<ItemState>;
  switch (change) {
    case "favorite":
    case "unfavorite":
      call = LibraryService.Favorite(ref, change === "favorite");
      break;
    case "suggest-less":
    case "undo-suggest-less":
      call = LibraryService.SuggestLess(ref, change === "suggest-less");
      break;
    case "remove":
      call = LibraryService.Remove(ref);
      break;
  }
  try {
    const next = await call;
    qc().setQueryData(key, next);
    if (next.inLibrary !== base.inLibrary || change === "remove") {
      for (const k of membershipLists) void qc().invalidateQueries({ queryKey: k });
    } else if (kind === "songs") {
      void qc().invalidateQueries({ queryKey: ["library", "favorite-songs"] });
    }
    return next;
  } catch (err) {
    qc().setQueryData(key, prev);
    throw err;
  }
}

/**
 * Adds catalog items to the library. Go reports them in the library at
 * once (with their library IDs) and syncs the affected lists afterwards.
 */
export async function addItemsToLibrary(
  kind: "songs" | "albums" | "playlists" | "music-videos",
  items: StateItem[],
): Promise<void> {
  await LibraryService.Add(
    kind,
    items.map((i) => i.catalogId || i.id),
  );
  for (const i of items) {
    qc().setQueryData<ItemState>(keys.itemState(kind, i.id), (s) =>
      s ? { ...s, inLibrary: true } : s,
    );
    void qc().invalidateQueries({ queryKey: keys.itemState(kind, i.id) });
  }
  for (const k of membershipLists) void qc().invalidateQueries({ queryKey: k });
}
