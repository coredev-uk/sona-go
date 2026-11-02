import type { Item, Section } from "@bindings/services";
import { t } from "@lingui/core/macro";

/**
 * Where an editorial section of Apple's radio page goes on Sona's: "live"
 * with the live stations at the top (local broadcasters, or Apple's own
 * live shelves when Sona couldn't list those), then the bands in this
 * order: stations to tune to, shows and episodes to listen back to, and
 * the ways to browse further.
 */
export type Place = "live" | "stations" | "shows" | "browse";
export const bandIds = ["stations", "shows", "browse"] as const;
export type BandId = (typeof bandIds)[number];

export interface Band {
  id: BandId;
  sections: Section[];
}

/** A band's heading. */
export function bandTitle(id: BandId): string {
  switch (id) {
    case "stations":
      return t`Stations`;
    case "shows":
      return t`Shows and episodes`;
    case "browse":
      return t`Browse`;
  }
}

function itemPlace(item: Item): Place {
  switch (item.kind) {
    case "station":
      switch (item.station?.kind) {
        case "live":
        case "broadcast":
          return "live";
        case "episode":
          return "shows";
        default:
          return "stations";
      }
    case "room":
    case "multiroom":
    case "grouping":
    case "charts":
    case "link":
      return "browse";
    default:
      // Shows (curators), interviews (videos), sessions and DJ mixes (albums).
      return "shows";
  }
}

/** Where a section goes: by its kind, else by most of its items (the first item's place breaks a tie). */
export function sectionPlace(section: Section): Place {
  switch (section.kind) {
    case "live":
    case "stations":
      return "live";
    case "bricks":
    case "links":
      return "browse";
  }
  const counts = new Map<Place, number>();
  for (const item of section.items) {
    const place = itemPlace(item);
    counts.set(place, (counts.get(place) ?? 0) + 1);
  }
  let best: Place = "shows";
  let most = 0;
  // A Map keeps insertion order, so on a tie the place seen first wins.
  for (const [place, n] of counts) {
    if (n > most) [best, most] = [place, n];
  }
  return best;
}
/**
 * Apple's radio sections placed: the live ones, then every band in
 * `bandIds` order (an empty one too, as the Stations band also holds the
 * user's own stations), Apple's order kept inside each. A paragraph goes
 * with the section after it (it introduces it); a trailing one with the
 * last section.
 */
export function placeSections(sections: readonly Section[]): { live: Section[]; bands: Band[] } {
  const byPlace = new Map<Place, Section[]>();
  let last: Place | undefined;
  let pending: Section[] = [];
  for (const section of sections) {
    if (section.kind === "text") {
      pending.push(section);
      continue;
    }
    last = sectionPlace(section);
    byPlace.set(last, [...(byPlace.get(last) ?? []), ...pending, section]);
    pending = [];
  }
  if (pending.length) {
    const place = last ?? "shows";
    byPlace.set(place, [...(byPlace.get(place) ?? []), ...pending]);
  }
  return {
    live: byPlace.get("live") ?? [],
    bands: bandIds.map((id) => ({ id, sections: byPlace.get(id) ?? [] })),
  };
}
