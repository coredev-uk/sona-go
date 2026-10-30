import { toast } from "sonner";

import { callFailure } from "./errors";

/**
 * Runs a fire-and-forget command (playback, library edits). A failure
 * becomes a toast naming what failed and why; nothing is retried.
 */
export function run(what: string, p: Promise<unknown>): Promise<boolean> {
  return p.then(
    () => true,
    (err) => {
      toast.error(what, { description: callFailure(err).message });
      return false;
    },
  );
}
