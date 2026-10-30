import type { Equalizer } from "@bindings/config";
import type { EqualizerPreset } from "@bindings/services";
import { t } from "@lingui/core/macro";

import { numberFormat } from "@/shared/lib/format";

export const CUSTOM = "custom";

/** The curve the setting describes: the selected preset's, or the user's own. */
export function gainsOf(eq: Equalizer, presets: readonly EqualizerPreset[]): number[] {
  const p = presets.find((p) => p.id === eq.preset);
  return p ? p.gains : eq.custom;
}

/**
 * eq with band `i` at `db`. Editing a preset starts the user's own curve
 * from it; the preset itself never changes.
 */
export function withBand(
  eq: Equalizer,
  presets: readonly EqualizerPreset[],
  i: number,
  db: number,
): Equalizer {
  const custom = [...gainsOf(eq, presets)];
  custom[i] = db;
  return { ...eq, preset: CUSTOM, custom };
}

/** The automatic attenuation (dB, ≤ 0) that keeps the largest boost from clipping. */
export function headroom(gains: readonly number[]) {
  return -Math.max(0, ...gains);
}

/** The level change ahead of the filters, as Go applies it (config.Equalizer.Pregain). */
export function pregainOf(eq: Equalizer, gains: readonly number[]) {
  return eq.enabled ? headroom(gains) + eq.preamp : 0;
}

/** "+3", "0", "−4.5": a dB value as the faders label it, in the active locale's digits. */
export function formatDb(db: number) {
  const r = Math.round(db * 2) / 2;
  if (r === 0) return "0";
  return (r > 0 ? "+" : "−") + numberFormat({ maximumFractionDigits: 1 }).format(Math.abs(r));
}

/** "29", "947", "1.9k", "15k": a band's frequency label (SI "k" for kHz). */
export function formatHz(hz: number) {
  if (hz < 1000) return numberFormat({ maximumFractionDigits: 0 }).format(hz);
  const k = hz / 1000;
  return numberFormat({ maximumFractionDigits: k < 10 ? 1 : 0 }).format(k) + "k";
}

/**
 * A preset's name in the active locale. Go names its built-in curves in
 * English (internal/playback/equalizer); an ID this list lacks keeps Go's name.
 */
export function presetName(p: Pick<EqualizerPreset, "id" | "name">): string {
  switch (p.id) {
    case CUSTOM:
      return t`Your curve`;
    case "flat":
      return t({ message: "Flat", comment: "Equaliser preset" });
    case "bass-boost":
      return t`Bass Boost`;
    case "bass-reducer":
      return t`Bass Reducer`;
    case "treble-boost":
      return t`Treble Boost`;
    case "treble-reducer":
      return t`Treble Reducer`;
    case "vocal":
      return t({ message: "Vocal", comment: "Equaliser preset" });
    case "loudness":
      return t({ message: "Loudness", comment: "Equaliser preset" });
    case "acoustic":
      return t({ message: "Acoustic", comment: "Equaliser preset" });
    case "electronic":
      return t({ message: "Electronic", comment: "Equaliser preset" });
    case "hip-hop":
      return t({ message: "Hip-Hop", comment: "Equaliser preset" });
    case "rock":
      return t({ message: "Rock", comment: "Equaliser preset" });
    case "classical":
      return t({ message: "Classical", comment: "Equaliser preset" });
    case "jazz":
      return t({ message: "Jazz", comment: "Equaliser preset" });
    case "piano":
      return t({ message: "Piano", comment: "Equaliser preset" });
    case "spoken-word":
      return t`Spoken Word`;
  }
  return p.name;
}
