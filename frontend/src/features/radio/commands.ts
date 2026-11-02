import { RadioService } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { toast } from "sonner";

import { replaceQueue } from "@/features/player";
import { callFailure } from "@/shared/lib/errors";

/** Create Station seeds, as Go's RadioService.CreateStation takes them. */
export type StationSeed = "song" | "artist" | "album";

function failed(what: string, err: unknown) {
  const { code, message } = callFailure(err);
  const description =
    code === "not_signed_in" || code === "mut_rejected"
      ? t`Sign in to Apple Music from the account menu in the top bar.`
      : code === "not_found"
        ? t`Apple Music has no station for this.`
        : message;
  toast.error(what, { description });
}

/**
 * Plays station `id` ("ra.…"): a track station fills the queue as it
 * plays; a live station or episode plays as one stream. Either replaces
 * the queue (with Undo over a long one the user built).
 */
export function playStation(id: string): Promise<boolean> {
  return replaceQueue(async () => {
    try {
      await RadioService.Play(id);
      return true;
    } catch (err) {
      failed(t`Couldn't play this station`, err);
      return false;
    }
  });
}

/** Starts the station seeded by a catalogue song, artist or album. */
export async function createStation(seed: StationSeed, catalogId: string, name: string) {
  try {
    const st = await RadioService.CreateStation(seed, catalogId);
    const station = st.name;
    toast(t`Playing ${station}`);
  } catch (err) {
    failed(t`Couldn't start a station from ${name}`, err);
  }
}
