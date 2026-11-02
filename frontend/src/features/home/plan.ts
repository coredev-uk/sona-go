import type { Item, Section } from "@bindings/services";

/** Plain cover shelves shown in full; later ones are listed in the index. */
export const OPEN_SHELVES = 3;

/** Apple's source for the Recently Played shelf (Section.source). */
export const RECENTLY_PLAYED = "recently-played";

/**
 * How Home sets Apple's shelves: the lead (the first "feature" shelf, Top
 * Picks), the shelves shown in full in Apple's order, and the index (plain
 * cover shelves after the first OPEN_SHELVES, listed one line each).
 * Recently Played, station shelves, features, heroes and track shelves are
 * always shown in full.
 */
export interface HomePlan {
  lead?: Section;
  open: Section[];
  index: Section[];
}

export function planHome(sections: readonly Section[]): HomePlan {
  const lead = sections.find((s) => s.kind === "feature" && s.items.length > 0);
  const open: Section[] = [];
  const index: Section[] = [];
  let plain = 0;
  for (const s of sections) {
    if (s === lead) continue;
    const plainShelf = s.kind === "shelf" && s.source !== RECENTLY_PLAYED && !isStations(s);
    if (plainShelf && plain++ >= OPEN_SHELVES) index.push(s);
    else open.push(s);
  }
  return { lead, open, index };
}

/** A shelf of stations only (Stations for You, moods, favourite artists' stations). */
export function isStations(s: Section) {
  return s.items.length > 0 && s.items.every((i) => i.kind === "station");
}

/**
 * The shelf's items without a caption every one of them shares ("Apple
 * Music for You" under each Made for You playlist): said once, it is noise.
 */
export function withoutSharedCaption(items: readonly Item[]): readonly Item[] {
  const first = items[0]?.caption;
  if (items.length < 2 || !first || items.some((i) => i.caption !== first)) return items;
  return items.map((i) => ({ ...i, caption: "" }));
}
