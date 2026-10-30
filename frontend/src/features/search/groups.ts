import type { Item } from "@bindings/services";

/**
 * Search result groups: Apple's group keys (SearchSection.group) and how
 * Sona titles and lays them out. Apple's own group names are not shown.
 */

const titles: Record<string, string> = {
  top: "Top results",
  artist: "Artists",
  album: "Albums",
  song: "Songs",
  playlist: "Playlists",
  radio_show: "Radio shows",
  radio_episode: "Radio episodes",
  station: "Stations",
  music_video: "Music videos",
  category: "Categories",
  curator: "Curators",
  record_label: "Labels",
  profile: "Profiles",
  shared_playlist: "Playlists by listeners",
};

/** The section title for a group; unknown groups get their key in sentence case. */
export function groupTitle(group: string): string {
  const t = titles[group];
  if (t) return t;
  const words = group.replace(/[_-]+/g, " ").trim();
  return words ? words[0].toUpperCase() + words.slice(1) + "s" : "Results";
}

/** Groups with a "see all" page (Top results is a fixed set). */
export function hasSeeAll(group: string, next: number, shown: number, total: number): boolean {
  return group !== "top" && (next > 0 || total > shown);
}

/** The noun for one item of a kind, for the "Album · Artist" line of mixed lists. */
export function kindLabel(kind: string, station?: { kind: string }): string {
  switch (kind) {
    case "song":
      return "Song";
    case "album":
      return "Album";
    case "artist":
      return "Artist";
    case "playlist":
      return "Playlist";
    case "station":
      if (station?.kind === "episode") return "Radio episode";
      return station?.kind === "live" || station?.kind === "broadcast" ? "Live radio" : "Station";
    case "music-video":
      return "Music video";
    case "curator":
      return "Curator";
    case "profile":
      return "Profile";
    case "room":
    case "multiroom":
    case "grouping":
      return "Collection";
    case "charts":
      return "Charts";
    default:
      return "";
  }
}

/** "Song · Artist": the item's kind, then its own subtitle. */
export function itemMeta(item: Item): string {
  return [kindLabel(item.kind, item.station ?? undefined), item.subtitle]
    .filter(Boolean)
    .join(" · ");
}
