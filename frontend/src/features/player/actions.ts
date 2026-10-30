import { ListEnd, ListStart } from "lucide-react";

import type { ActionDef } from "@/shared/actions/registry";

import { addToQueue, playNext } from "./commands";

/** Song row actions: queue the song next or last. */
export const playerActions: ActionDef<"song">[] = [
  {
    id: "player.play-next",
    target: "song",
    label: "Play next",
    icon: ListStart,
    group: "queue",
    order: 10,
    isEnabled: (song) => song.playable,
    run: (song) => playNext([song]),
  },
  {
    id: "player.add-to-queue",
    target: "song",
    label: "Add to queue",
    icon: ListEnd,
    group: "queue",
    order: 20,
    isEnabled: (song) => song.playable,
    run: (song) => addToQueue([song]),
  },
];
