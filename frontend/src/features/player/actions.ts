import { t } from "@lingui/core/macro";
import { ListEnd, ListStart } from "lucide-react";

import type { ActionDef, ActionItem, BulkActionDef } from "@/shared/actions/registry";

import { queueItems } from "./commands";

/** Selected songs can be queued when one of them plays; collections are checked when loaded. */
const somePlayable = (items: readonly ActionItem[]) =>
  items.some((i) => i.kind !== "song" || i.target.playable);

/**
 * Queue actions: a song row queues next or last; a selection of songs,
 * albums and playlists queues all their songs in order.
 */
export const playerActions: (ActionDef<"song"> | BulkActionDef)[] = [
  {
    id: "player.play-next",
    target: "song",
    label: () => t`Play next`,
    icon: ListStart,
    group: "queue",
    order: 10,
    isEnabled: (song) => song.playable,
    run: (song) => void queueItems([{ kind: "song", target: song }], "next"),
  },
  {
    id: "player.add-to-queue",
    target: "song",
    label: () => t`Add to queue`,
    icon: ListEnd,
    group: "queue",
    order: 20,
    isEnabled: (song) => song.playable,
    run: (song) => void queueItems([{ kind: "song", target: song }], "last"),
  },
  {
    id: "player.play-next.many",
    target: "selection",
    kinds: ["song", "album", "playlist"],
    label: () => t`Play next`,
    icon: ListStart,
    group: "queue",
    order: 10,
    isEnabled: somePlayable,
    run: (items) => void queueItems(items, "next"),
  },
  {
    id: "player.add-to-queue.many",
    target: "selection",
    kinds: ["song", "album", "playlist"],
    label: () => t`Add to queue`,
    icon: ListEnd,
    group: "queue",
    order: 20,
    isEnabled: somePlayable,
    run: (items) => void queueItems(items, "last"),
  },
];
