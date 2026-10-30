import { RadioService } from "@bindings/services";
import { toast } from "sonner";

import { callFailure } from "@/shared/lib/errors";

/** Create Station seeds, as Go's RadioService.CreateStation takes them. */
export type StationSeed = "song" | "artist" | "album";

function failed(what: string, err: unknown) {
  const { code, message } = callFailure(err);
  const description =
    code === "not_signed_in" || code === "mut_rejected"
      ? "Sign in to Apple Music in Settings → Account."
      : code === "not_found"
        ? "Apple Music has no station for this."
        : message;
  toast.error(what, { description });
}

/**
 * Plays station `id` ("ra.…"): a track station fills the queue as it
 * plays; a live station or episode plays as one stream.
 */
export async function playStation(id: string): Promise<boolean> {
  try {
    await RadioService.Play(id);
    return true;
  } catch (err) {
    failed("Couldn't play this station", err);
    return false;
  }
}

/** Starts the station seeded by a catalogue song, artist or album. */
export async function createStation(seed: StationSeed, catalogId: string, name: string) {
  try {
    const st = await RadioService.CreateStation(seed, catalogId);
    toast(`Playing ${st.name}`);
  } catch (err) {
    failed(`Couldn't start a station from ${name}`, err);
  }
}
