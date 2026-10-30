import { describe, expect, it } from "vitest";

import { logFrequencies, logPosition, response } from "./response";

const freqs = [29, 59, 119, 237, 474, 947, 1889, 3770, 7523, 15011];
const flat = freqs.map(() => 0);

describe("response", () => {
  it("is flat at the pregain when every band is 0 dB", () => {
    for (const db of response(freqs, flat, -3, logFrequencies(40))) expect(db).toBeCloseTo(-3, 9);
  });

  it("peaks at a mid band's centre by its gain and fades away from it", () => {
    const gains = flat.map((_, i) => (i === 5 ? 6 : 0));
    const [atCentre, oneOctaveUp, farAway] = response(freqs, gains, 0, [947, 1894, 15000]);
    expect(atCentre).toBeCloseTo(6, 1);
    expect(oneOctaveUp).toBeGreaterThan(0);
    expect(oneOctaveUp).toBeLessThan(atCentre);
    expect(Math.abs(farAway)).toBeLessThan(0.1);
  });

  it("shelves the lowest band below its frequency and the highest above", () => {
    const low = response(
      freqs,
      flat.map((_, i) => (i === 0 ? -9 : 0)),
      0,
      [20, 5000],
    );
    expect(low[0]).toBeLessThan(-6);
    expect(Math.abs(low[1])).toBeLessThan(0.1);
    const high = response(
      freqs,
      flat.map((_, i) => (i === 9 ? 9 : 0)),
      0,
      [20_000, 100],
    );
    expect(high[0]).toBeGreaterThan(6);
    expect(Math.abs(high[1])).toBeLessThan(0.1);
  });
});

describe("logPosition", () => {
  it("places the band centres evenly, as the faders sit", () => {
    const pos = freqs.map(logPosition);
    const gaps = pos.slice(1).map((p, i) => p - pos[i]);
    for (const g of gaps) expect(g).toBeCloseTo(0.1, 2);
  });
});
