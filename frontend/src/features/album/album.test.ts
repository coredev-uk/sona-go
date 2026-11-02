import { Song } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { discsOf, isCompilation, libraryShare, withoutPlaceholderCredits } from "./album";

const song = (p: Partial<Song>) =>
  new Song({ id: String(Math.random()), discNumber: 1, playable: true, ...p });

describe("album page", () => {
  it("indexes discs by where they start in the list, with their songs and running time", () => {
    const list = [
      song({ discNumber: 1, durationMs: 1000 }),
      song({ discNumber: 1, durationMs: 2000 }),
      song({ discNumber: 2, durationMs: 500 }),
    ];
    expect(discsOf(list)).toEqual([
      { number: 1, first: 0, songs: 2, durationMs: 3000 },
      { number: 2, first: 2, songs: 1, durationMs: 500 },
    ]);
    // One disc needs no index.
    expect(discsOf(list.slice(0, 2))).toEqual([]);
  });

  it("gives an Artist column to compilations, not to an album with a featured artist", () => {
    const own = song({ artistName: "Zara Larsson" });
    const feat = song({ artistName: "Zara Larsson & David Guetta" });
    expect(isCompilation([own, own, own, feat], "Zara Larsson")).toBe(false);
    expect(
      isCompilation([song({ artistName: "A" }), song({ artistName: "B" })], "Various Artists"),
    ).toBe(true);
  });

  it("drops the stand-in credit of an unpublished song on an album not out yet", () => {
    const placeholder = song({
      name: "Track 3",
      artistName: "unknown",
      playable: false,
      durationMs: 0,
    });
    const announced = song({ artistName: "Morgan Wallen", playable: false, durationMs: 196_000 });
    const out = song({ artistName: "Yung Lean", durationMs: 161_000 });
    const [a, b, c] = withoutPlaceholderCredits([placeholder, announced, out], true);
    expect(a.artistName).toBe("");
    expect(b.artistName).toBe("Morgan Wallen");
    expect(c).toBe(out);
    // Once out, songs keep whatever Apple credits them with.
    expect(withoutPlaceholderCredits([placeholder], false)[0]).toBe(placeholder);
  });

  it("tells a partial library album from a whole one", () => {
    expect(libraryShare(2, 13)).toEqual({ have: 2, total: 13 });
    expect(libraryShare(13, 13)).toBeUndefined();
    expect(libraryShare(2, undefined)).toBeUndefined();
  });
});
