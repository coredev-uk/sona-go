import { Song } from "@bindings/services";
import { describe, expect, it } from "vitest";

import {
  discsOf,
  firstReleased,
  hasWorks,
  isCompilation,
  libraryShare,
  movementTitle,
  withoutPlaceholderCredits,
  workHeading,
  writersOf,
} from "./album";

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

  it("doesn't count songs with no credit yet towards a compilation", () => {
    // An album not out yet: most songs are placeholders with their credit dropped.
    const out = song({ artistName: "Kings of Leon" });
    const unpublished = song({ artistName: "" });
    expect(isCompilation([out, out, unpublished, unpublished, unpublished], "Kings of Leon")).toBe(
      false,
    );
  });

  it("lists each songwriter once, the most credited first", () => {
    const list = [
      song({ composerName: "Christine McVie" }),
      song({ composerName: "Lindsey Buckingham" }),
      song({ composerName: "Lindsey Buckingham & Stevie Nicks" }),
      song({
        composerName:
          "Christine McVie, John McVie, Lindsey Buckingham, Mick Fleetwood & Stevie Nicks",
      }),
      song({ composerName: "" }),
    ];
    expect(writersOf(list)).toEqual([
      "Lindsey Buckingham",
      "Christine McVie",
      "Stevie Nicks",
      "John McVie",
      "Mick Fleetwood",
    ]);
  });

  it("groups a classical album's movements under their works", () => {
    const aria = song({
      name: "Goldberg Variations, BWV 988 (1955 Recording): Aria",
      workName: "Goldberg Variations, BWV 988 (1955 Recording)",
      movementName: "Aria",
      composerName: "Johann Sebastian Bach",
    });
    const var1 = song({ ...aria, id: "v1", name: "…: Variation 1", movementName: "Variation 1" });
    const later = song({
      ...aria,
      id: "l",
      workName: "Goldberg Variations, BWV 988 (1981 Recording)",
    });
    const pop = song({ name: "Dreams" });
    expect(hasWorks([pop, aria])).toBe(true);
    expect(hasWorks([pop])).toBe(false);
    expect(workHeading(aria, undefined, 1)).toBe("Goldberg Variations, BWV 988 (1955 Recording)");
    expect(workHeading(var1, aria, 1)).toBeUndefined();
    expect(workHeading(later, var1, 1)).toBe("Goldberg Variations, BWV 988 (1981 Recording)");
    // A recital of several composers names the composer with the work.
    expect(workHeading(aria, pop, 2)).toBe(
      "Johann Sebastian Bach · Goldberg Variations, BWV 988 (1955 Recording)",
    );
    // A new disc starts the work again.
    expect(workHeading(song({ ...var1, discNumber: 2 }), aria, 1)).toBeDefined();
    expect(workHeading(pop, aria, 1)).toBeUndefined();
    expect(movementTitle(var1)).toBe("Variation 1");
    expect(movementTitle(pop)).toBe("Dreams");
  });

  it("says when the recordings are older than the album", () => {
    const list = [song({ releaseDate: "1982-09-02" }), song({ releaseDate: "1959-01-01" })];
    expect(firstReleased(list, "2013-01-28")).toBe("1959");
    expect(firstReleased(list, "1959-05-01")).toBe("");
    expect(firstReleased([song({ releaseDate: "" })], "2013")).toBe("");
    expect(firstReleased(list, "")).toBe("");
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
