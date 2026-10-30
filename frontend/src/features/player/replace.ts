import { PlaybackService, SystemService } from "@bindings/services";
import { plural, t } from "@lingui/core/macro";
import { toast } from "sonner";

import { confirm } from "@/shared/lib/confirm";
import { run } from "@/shared/lib/run";

import { usePlayback } from "./playback";

/**
 * More upcoming songs the user queued by hand than this, and replacing the
 * queue asks first (setting "confirm") or offers Undo (the default).
 */
export const LONG_USER_QUEUE = 3;

/** The upcoming songs the user queued by hand (Play Next, Add to Queue). */
export function addedUpcoming(): number {
  const { items, index } = usePlayback.getState().queue;
  let n = 0;
  for (let i = index + 1; i < items.length; i++) if (items[i].added) n++;
  return n;
}

/**
 * Starts a new queue with `start` (playing an album, playlist, station…),
 * which reports its own failure and resolves whether playback started.
 * When that would discard a long queue the user built, it asks first or,
 * by default, replaces it and offers Undo, which brings back the old queue
 * at the same song and position. Resolves whether playback started.
 */
export async function replaceQueue(start: () => Promise<boolean>): Promise<boolean> {
  const added = addedUpcoming();
  if (added <= LONG_USER_QUEUE) return start();
  const mode = await SystemService.Settings().then(
    (s) => s.queue.replaceConfirm,
    () => "undo",
  );
  if (
    mode === "confirm" &&
    !(await confirm({
      title: t`Replace your queue?`,
      body: plural(added, {
        one: "The # song you added to the queue will be removed.",
        other: "The # songs you added to the queue will be removed.",
      }),
      confirmLabel: t`Replace queue`,
    }))
  ) {
    return false;
  }
  if (!(await start())) return false;
  if (mode !== "confirm") {
    toast(t`Queue replaced`, {
      description: plural(added, {
        one: "# song you added was removed.",
        other: "# songs you added were removed.",
      }),
      duration: 10_000,
      action: {
        label: t`Undo`,
        onClick: () => void run(t`Couldn't bring back your queue`, PlaybackService.UndoReplace()),
      },
    });
  }
  return true;
}
