import type { Equalizer } from "@bindings/config";
import { AudioService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

export const equalizerInfoKey = ["audio", "equalizer"] as const;

/** Bands, limits and presets: fixed for the app's lifetime. */
export function useEqualizerInfo() {
  return useQuery({
    queryKey: equalizerInfoKey,
    queryFn: () => AudioService.Equalizer(),
    staleTime: Infinity,
    gcTime: Infinity,
  });
}

let pending: Equalizer | null = null;
let sending = false;

/**
 * Sends a curve to the playing audio while a control moves, one call at a
 * time: curves that arrive meanwhile collapse into the latest, which always
 * goes out last.
 */
export function preview(eq: Equalizer) {
  pending = eq;
  if (sending) return;
  sending = true;
  void (async () => {
    while (pending) {
      const next = pending;
      pending = null;
      await AudioService.PreviewEqualizer(next).catch(() => {});
    }
    sending = false;
  })();
}
