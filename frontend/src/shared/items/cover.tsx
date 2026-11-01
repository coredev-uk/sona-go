import type { Item } from "@bindings/services";
import { Browser } from "@wailsio/runtime";

import type { CoverItem } from "@/shared/components/CoverGrid";
import { releaseBadge } from "@/shared/lib/release";

import { actionItemOf, itemAspect, itemLink, itemShape, itemSubtitle } from "./links";
import { canPlay, playItem } from "./play";

/** Opens a web-only item (an interview, a concert listing) in the browser. */
export function openExternal(item: Item) {
  if (item.url) void Browser.OpenURL(item.url);
}

/**
 * An Item as a cover tile: its page, a play control when it plays
 * directly, an upcoming album's release date, and its "more" menu.
 * `onOpen` runs whenever the tile is opened (search remembers opened
 * items).
 */
export function toCoverItem(item: Item, index: number, onOpen?: (item: Item) => void): CoverItem {
  const link = itemLink(item);
  return {
    key: `${index}:${item.kind}:${item.id || item.url}`,
    title: item.name,
    subtitle: itemSubtitle(item),
    art: item.art.medium || item.wide.medium,
    motion: (item.album ?? item.playlist ?? item.artist ?? item.station)?.motion || undefined,
    shape: itemShape(item),
    aspect: itemAspect(item),
    link,
    onOpen: link
      ? onOpen && (() => onOpen(item))
      : () => {
          onOpen?.(item);
          openExternal(item);
        },
    play: canPlay(item) ? { label: item.name, run: () => playItem(item) } : undefined,
    badge: item.album ? releaseBadge(item.album) : undefined,
    target: actionItemOf(item),
  };
}

/** The tiles' shared aspect: video when every item is a video frame. */
export function rowAspect(items: readonly Item[]): "square" | "video" {
  return items.length > 0 && items.every((i) => itemAspect(i) === "video") ? "video" : "square";
}
