import { t } from "@lingui/core/macro";
import { ListEnd, ListStart } from "lucide-react";

import type { ActionItem, ItemActionDef } from "@/shared/actions/registry";

import { unavailableNow } from "./availability";
import { queueItems } from "./commands";

/** A live station has no queue to add to. */
const noQueue = () => unavailableNow("queue");

/** Selected songs can be queued when one of them plays; collections are checked when loaded. */
const somePlayable = (items: readonly ActionItem[]) =>
  items.some((i) => i.kind !== "song" || i.target.playable);

/**
 * Queue actions: a song or music video queues next or last; a selection
 * of songs, music videos, albums and playlists queues them all in order
 * (a collection as its songs). Music videos have action targets only
 * where videos play.
 */
export const playerActions: ItemActionDef[] = [
  {
    id: "player.play-next",
    target: "song",
    label: () => t`Play next`,
    icon: ListStart,
    group: "queue",
    order: 10,
    disabledReason: noQueue,
    isEnabled: (song) => song.playable,
    run: (song) => void queueItems([{ kind: "song", target: song }], "next"),
  },
  {
    id: "player.add-to-queue",
    target: "song",
    label: () => t`Play last`,
    icon: ListEnd,
    group: "queue",
    order: 20,
    disabledReason: noQueue,
    isEnabled: (song) => song.playable,
    run: (song) => void queueItems([{ kind: "song", target: song }], "last"),
  },
  {
    id: "player.play-next.video",
    target: "music-video",
    label: () => t`Play next`,
    icon: ListStart,
    group: "queue",
    order: 10,
    disabledReason: noQueue,
    run: (video) => void queueItems([{ kind: "music-video", target: video }], "next"),
  },
  {
    id: "player.add-to-queue.video",
    target: "music-video",
    label: () => t`Play last`,
    icon: ListEnd,
    group: "queue",
    order: 20,
    disabledReason: noQueue,
    run: (video) => void queueItems([{ kind: "music-video", target: video }], "last"),
  },
  {
    id: "player.play-next.many",
    target: "selection",
    kinds: ["song", "music-video", "album", "playlist"],
    label: () => t`Play next`,
    icon: ListStart,
    group: "queue",
    order: 10,
    disabledReason: noQueue,
    isEnabled: somePlayable,
    run: (items) => void queueItems(items, "next"),
  },
  {
    id: "player.add-to-queue.many",
    target: "selection",
    kinds: ["song", "music-video", "album", "playlist"],
    label: () => t`Play last`,
    icon: ListEnd,
    group: "queue",
    order: 20,
    disabledReason: noQueue,
    isEnabled: somePlayable,
    run: (items) => void queueItems(items, "last"),
  },
];
