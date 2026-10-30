import { Source, type Item, type RecentSearch } from "@bindings/services";

import { itemLink, type ItemLinkProps } from "@/shared/items/links";

const noArt = { small: "", medium: "", large: "" };

/**
 * A remembered item as a card, enough to show it and open its page (the
 * page loads the rest). Library items open their library page by ID.
 */
export function recentItem(r: RecentSearch): Item {
  return {
    kind: r.kind,
    id: r.itemId,
    name: r.name,
    subtitle: r.subtitle,
    caption: "",
    art: r.art ? { small: r.art, medium: r.art, large: r.art } : noArt,
    wide: noArt,
    color: "",
    explicit: false,
    url: "",
  } as Item;
}

/** Whether a recent entry was a library item (its ID is a library ID). */
export function isLibraryEntry(r: RecentSearch): boolean {
  return r.kind !== "term" && r.source === Source.SourceLibrary;
}

/**
 * The page a remembered item opens. Songs open their song page (a recent
 * entry does not keep the album a song result linked to); library songs
 * have none.
 */
export function recentLink(r: RecentSearch): ItemLinkProps | undefined {
  if (r.kind !== "song") return itemLink(recentItem(r));
  return isLibraryEntry(r) ? undefined : { to: "/song/$id", params: { id: r.itemId } };
}
