/**
 * The equaliser's frequency response, computed like GStreamer's
 * equalizer-10bands (gstiirequalizer.c): band 0 is a low shelf, the last
 * band a high shelf, the others peaking filters, each as wide as its slice
 * of 20 Hz–20 kHz on a log scale. Used to draw the curve the playback host
 * really applies.
 */

const RATE = 48_000;
const LOWEST = 20;
const HIGHEST = 20_000;

interface Biquad {
  /** Numerator b0..b2 and denominator a0..a2 (a0 = 1 after normalising). */
  b: [number, number, number];
  a: [number, number, number];
}

/** The width in Hz of band `i` of `n` (gstiirequalizernbands.c). */
function bandWidth(i: number, n: number) {
  const step = Math.pow(HIGHEST / LOWEST, 1 / n);
  const f0 = LOWEST * Math.pow(step, i);
  return f0 * step - f0;
}

function filter(
  kind: "peak" | "low" | "high",
  freq: number,
  width: number,
  gainDb: number,
): Biquad {
  const g = Math.pow(10, gainDb / 40);
  const w = (2 * Math.PI * freq) / RATE;
  const cos = Math.cos(w);
  const alpha = Math.tan((Math.PI * width) / RATE);
  if (kind === "peak") {
    const d = 1 + alpha / g;
    return {
      b: [(1 + alpha * g) / d, (-2 * cos) / d, (1 - alpha * g) / d],
      a: [1, (-2 * cos) / d, (1 - alpha / g) / d],
    };
  }
  const egm = g - 1;
  const egp = g + 1;
  const delta = 2 * Math.sqrt(g) * alpha;
  if (kind === "low") {
    const d = egp + egm * cos + delta;
    return {
      b: [
        ((egp - egm * cos + delta) * g) / d,
        ((egm - egp * cos) * 2 * g) / d,
        ((egp - egm * cos - delta) * g) / d,
      ],
      a: [1, (-(egm + egp * cos) * 2) / d, (egp + egm * cos - delta) / d],
    };
  }
  const d = egp - egm * cos + delta;
  return {
    b: [
      ((egp + egm * cos + delta) * g) / d,
      ((egm + egp * cos) * -2 * g) / d,
      ((egp + egm * cos - delta) * g) / d,
    ],
    a: [1, ((egm - egp * cos) * 2) / d, (egp - egm * cos - delta) / d],
  };
}

/** |H(e^jw)| of one biquad, in dB. */
function magnitudeDb(f: Biquad, freq: number) {
  const w = (2 * Math.PI * freq) / RATE;
  const c1 = Math.cos(w);
  const s1 = Math.sin(w);
  const c2 = Math.cos(2 * w);
  const s2 = Math.sin(2 * w);
  const nr = f.b[0] + f.b[1] * c1 + f.b[2] * c2;
  const ni = -(f.b[1] * s1 + f.b[2] * s2);
  const dr = f.a[0] + f.a[1] * c1 + f.a[2] * c2;
  const di = -(f.a[1] * s1 + f.a[2] * s2);
  return 10 * Math.log10((nr * nr + ni * ni) / (dr * dr + di * di));
}

/**
 * The response in dB at each frequency in `at` for band gains `gains`
 * (centred on `freqs`), including `pregainDb`.
 */
export function response(
  freqs: readonly number[],
  gains: readonly number[],
  pregainDb: number,
  at: readonly number[],
): number[] {
  const n = freqs.length;
  const filters = freqs.flatMap((f, i) =>
    gains[i]
      ? [filter(i === 0 ? "low" : i === n - 1 ? "high" : "peak", f, bandWidth(i, n), gains[i])]
      : [],
  );
  return at.map((f) => filters.reduce((db, bq) => db + magnitudeDb(bq, f), pregainDb));
}

/** `count` frequencies spaced evenly on a log scale over 20 Hz–20 kHz. */
export function logFrequencies(count: number): number[] {
  return Array.from(
    { length: count },
    (_, i) => LOWEST * Math.pow(HIGHEST / LOWEST, i / (count - 1)),
  );
}

/** Where `freq` sits on the 20 Hz–20 kHz log axis, 0..1. */
export function logPosition(freq: number) {
  return Math.log(freq / LOWEST) / Math.log(HIGHEST / LOWEST);
}
