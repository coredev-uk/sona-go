import { t } from "@lingui/core/macro";

import type { Item } from "@bindings/services";

/**
 * Search result groups: Apple's group keys (SearchSection.group) and how
 * Sona titles and lays them out. Apple's own group names are not shown.
 */

const titles: Record<string, () => string> = {
  top: () => t`Top results`,
  artist: () => t`Artists`,
  album: () => t`Albums`,
  song: () => t`Songs`,
  playlist: () => t`Playlists`,
  radio_show: () => t`Radio shows`,
  radio_episode: () => t`Radio episodes`,
  station: () => t`Stations`,
  music_video: () => t`Music videos`,
  category: () => t`Categories`,
  curator: () => t`Curators`,
  record_label: () => t`Labels`,
  profile: () => t`Profiles`,
  shared_playlist: () => t`Playlists by listeners`,
};

/** The section title for a group; unknown groups get their key in sentence case. */
export function groupTitle(group: string): string {
  const title = titles[group];
  if (title) return title();
  const words = group.replace(/[_-]+/g, " ").trim();
  if (!words) return t`Results`;
  return words[0].toUpperCase() + words.slice(1) + "s";
}

/** The error title of a group's "see all" page. */
export function groupLoadError(group: string): string {
  switch (group) {
    case "top":
      return t`Couldn't load top results`;
    case "artist":
      return t`Couldn't load artists`;
    case "album":
      return t`Couldn't load albums`;
    case "song":
      return t`Couldn't load songs`;
    case "playlist":
      return t`Couldn't load playlists`;
    case "radio_show":
      return t`Couldn't load radio shows`;
    case "radio_episode":
      return t`Couldn't load radio episodes`;
    case "station":
      return t`Couldn't load stations`;
    case "music_video":
      return t`Couldn't load music videos`;
    case "category":
      return t`Couldn't load categories`;
    case "curator":
      return t`Couldn't load curators`;
    case "record_label":
      return t`Couldn't load labels`;
    case "profile":
      return t`Couldn't load profiles`;
    case "shared_playlist":
      return t`Couldn't load playlists by listeners`;
    default:
      return t`Couldn't load these results`;
  }
}

/** The empty-state title of a group's "see all" page. */
export function groupEmpty(group: string, q: string): string {
  switch (group) {
    case "top":
      return t`No top results for “${q}”`;
    case "artist":
      return t`No artists for “${q}”`;
    case "album":
      return t`No albums for “${q}”`;
    case "song":
      return t`No songs for “${q}”`;
    case "playlist":
      return t`No playlists for “${q}”`;
    case "radio_show":
      return t`No radio shows for “${q}”`;
    case "radio_episode":
      return t`No radio episodes for “${q}”`;
    case "station":
      return t`No stations for “${q}”`;
    case "music_video":
      return t`No music videos for “${q}”`;
    case "category":
      return t`No categories for “${q}”`;
    case "curator":
      return t`No curators for “${q}”`;
    case "record_label":
      return t`No labels for “${q}”`;
    case "profile":
      return t`No profiles for “${q}”`;
    case "shared_playlist":
      return t`No playlists by listeners for “${q}”`;
    default:
      return t`Nothing for “${q}”`;
  }
}

/** Groups with a "see all" page (Top results is a fixed set). */
export function hasSeeAll(group: string, next: number, shown: number, total: number): boolean {
  return group !== "top" && (next > 0 || total > shown);
}

/** The noun for one item of a kind, for the "Album · Artist" line of mixed lists. */
export function kindLabel(kind: string, station?: { kind: string }): string {
  switch (kind) {
    case "song":
      return t`Song`;
    case "album":
      return t`Album`;
    case "artist":
      return t`Artist`;
    case "playlist":
      return t`Playlist`;
    case "station":
      if (station?.kind === "episode") return t`Radio episode`;
      return station?.kind === "live" || station?.kind === "broadcast" ? t`Live radio` : t`Station`;
    case "music-video":
      return t`Music video`;
    case "curator":
      return t`Curator`;
    case "profile":
      return t`Profile`;
    case "room":
    case "multiroom":
    case "grouping":
      return t`Collection`;
    case "charts":
      return t`Charts`;
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
