import type { Item, Section } from "@bindings/services";
import type { LinkProps } from "@tanstack/react-router";

import type { ActionItem } from "@/shared/actions/registry";

import { canPlayVideo } from "./play";

export type ItemLinkProps = Pick<LinkProps, "to" | "params" | "search">;

/**
 * The Sona page an item opens, by kind; `undefined` for items that have
 * none: those that exist only on the web (kind "link", opened with
 * `openWithoutPage`) and stations with nothing to show on a page (a local
 * broadcaster has no schedule), which play when they are opened.
 */
export function itemLink(item: Item): ItemLinkProps | undefined {
  const params = { id: item.id };
  switch (item.kind) {
    case "album":
      return { to: "/album/$id", params };
    case "playlist":
      return { to: "/playlist/$id", params };
    case "artist":
      return { to: "/artist/$id", params };
    case "song":
      return { to: "/song/$id", params };
    case "station":
      return item.station && !item.station.hasPage ? undefined : { to: "/station/$id", params };
    case "music-video":
      return { to: "/music-video/$id", params };
    case "curator":
      return { to: "/curator/$id", params };
    case "room":
      return { to: "/room/$id", params };
    case "multiroom":
      return { to: "/multiroom/$id", params };
    case "grouping":
      return { to: "/grouping/$id", params };
    case "charts":
      return { to: "/charts", search: item.id ? { genre: item.id } : {} };
    case "profile":
      return { to: "/profile/$id", params };
    case "concerts":
      return { to: "/concerts" };
    default:
      return undefined;
  }
}

/** Artists and profiles are round; videos are 16:9; everything else square. */
export function itemShape(item: Item): "square" | "round" {
  return item.kind === "artist" || item.kind === "profile" ? "round" : "square";
}

export function itemAspect(item: Item): "square" | "video" {
  return item.kind === "music-video" ||
    (item.kind === "link" && !!item.art.medium && isVideoUrl(item.url))
    ? "video"
    : "square";
}

function isVideoUrl(url: string) {
  return /\/(post|music-movie)\//.test(url);
}

/**
 * The line under an item's title: Apple's reason or badge, then the
 * artist, curator or host ("New release · Radiohead").
 */
export function itemSubtitle(item: Item): string {
  return [item.caption, item.subtitle].filter(Boolean).join(" · ");
}

/** A section's "See all": its editorial room, when it has one. */
export function roomLink(section: Section): ItemLinkProps | undefined {
  return section.room ? { to: "/room/$id", params: { id: section.room } } : undefined;
}

/** What an Item is for actions, selection and drag; undefined for kinds without actions. */
export function actionItemOf(item: Item): ActionItem | undefined {
  if (item.song) return { kind: "song", target: item.song };
  if (item.album) return { kind: "album", target: item.album };
  if (item.playlist) return { kind: "playlist", target: item.playlist };
  if (item.station) return { kind: "station", target: item.station };
  if (item.artist) return { kind: "artist", target: item.artist };
  if (item.musicVideo && canPlayVideo()) return { kind: "music-video", target: item.musicVideo };
  return undefined;
}
