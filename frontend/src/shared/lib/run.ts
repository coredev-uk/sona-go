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
      const { code, message } = callFailure(err);
      const detail =
        code === "not_signed_in" || code === "mut_rejected"
          ? "Sign in to Apple Music in Settings → Account."
          : code === "not_playable"
            ? "This song isn't available to stream in your storefront."
            : message;
      toast.error(what, { description: detail });
      return false;
    },
  );
}
