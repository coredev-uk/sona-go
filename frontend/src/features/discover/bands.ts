import { t } from "@lingui/core/macro";
import type { Item, Section } from "@bindings/services";

import { itemAspect } from "@/shared/items/links";

/**
 * Discover's bands: the page's shelves grouped by what they hold, in this
 * order, so the page reads as songs and albums, then playlists, radio,
 * videos and the ways to browse further.
 */
export const bandIds = ["music", "playlists", "radio", "video", "browse"] as const;
export type BandId = (typeof bandIds)[number];

export interface Band {
  id: BandId;
  sections: Section[];
}

/** A band's heading. */
export function bandTitle(id: BandId): string {
  switch (id) {
    case "music":
      return t`Songs and albums`;
    case "playlists":
      return t`Playlists`;
    case "radio":
      return t`Radio`;
    case "video":
      return t`Videos`;
    case "browse":
      return t`Browse`;
  }
}

function itemBand(item: Item): BandId {
  switch (item.kind) {
    case "album":
    case "song":
    case "artist":
      return "music";
    case "playlist":
      return "playlists";
    case "station":
      return "radio";
    case "music-video":
      return "video";
    default:
      // Curators, rooms, hubs and web links; a web link to a video is a video.
      return itemAspect(item) === "video" ? "video" : "browse";
  }
}

/** The band a section belongs to: by its kind, else by most of its items (the first item's band breaks a tie). */
export function sectionBand(section: Section): BandId {
  switch (section.kind) {
    case "tracks":
      return "music";
    case "live":
    case "stations":
      return "radio";
    case "bricks":
    case "links":
      return "browse";
  }
  const counts = new Map<BandId, number>();
  for (const item of section.items) {
    const band = itemBand(item);
    counts.set(band, (counts.get(band) ?? 0) + 1);
  }
  let best: BandId = "browse";
  let most = 0;
  // A Map keeps insertion order, so on a tie the band seen first wins.
  for (const [band, n] of counts) {
    if (n > most) [best, most] = [band, n];
  }
  return best;
}

/**
 * The page's sections in bands, in `bandIds` order, keeping Apple's order
 * inside each band. A paragraph goes with the section after it (it
 * introduces it); a trailing one with the last section.
 */
export function toBands(sections: readonly Section[]): Band[] {
  const byBand = new Map<BandId, Section[]>();
  let lastBand: BandId | undefined;
  let pending: Section[] = [];
  for (const section of sections) {
    if (section.kind === "text") {
      pending.push(section);
      continue;
    }
    lastBand = sectionBand(section);
    byBand.set(lastBand, [...(byBand.get(lastBand) ?? []), ...pending, section]);
    pending = [];
  }
  if (pending.length) {
    const band = lastBand ?? "browse";
    byBand.set(band, [...(byBand.get(band) ?? []), ...pending]);
  }
  return bandIds.flatMap((id) => {
    const found = byBand.get(id);
    return found ? [{ id, sections: found }] : [];
  });
}
