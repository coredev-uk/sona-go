import type { ArtistEntry } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { filterArtists, fold, indexArtists, listRows } from "./artists";

const entry = (name: string, albums = 1, songs = 10): ArtistEntry =>
  ({ id: `r.${name}`, catalogId: "", name, art: "", albums, songs }) as ArtistEntry;

const names = (list: { entry: ArtistEntry }[]) => list.map((a) => a.entry.name);

const library = indexArtists(
  [
    "Zara Larsson",
    "björk",
    "Beyoncé",
    "The Beatles",
    "Édith Piaf",
    "Eric Clapton",
    "AC/DC",
    "2Pac",
    "坂本龍一",
    "Sigur Rós",
    "Mötley Crüe",
    "Röyksopp",
    "Radiohead",
    "Band 10",
    "Band 9",
  ].map((n) => entry(n)),
);

describe("fold", () => {
  it("drops case and accents", () => {
    expect(fold("Sigur Rós")).toBe("sigur ros");
    expect(fold("BJÖRK")).toBe("bjork");
    expect(fold("Mötley Crüe")).toBe("motley crue");
    expect(fold("Ólafur Arnalds")).toBe("olafur arnalds");
  });

  it("folds letters Unicode does not decompose", () => {
    expect(fold("Sørensen")).toBe("sorensen");
    expect(fold("Straße")).toBe("strasse");
    expect(fold("Łódź")).toBe("lodz");
  });
});

describe("indexArtists", () => {
  it("orders A to Z without regard to case or accents, then everything else under #", () => {
    expect(names(library)).toEqual([
      "AC/DC",
      "Band 9",
      "Band 10",
      "Beyoncé",
      "björk",
      "Édith Piaf",
      "Eric Clapton",
      "Mötley Crüe",
      "Radiohead",
      "Röyksopp",
      "Sigur Rós",
      "The Beatles",
      "Zara Larsson",
      "2Pac",
      "坂本龍一",
    ]);
  });

  it("files accented names under their base letter", () => {
    const letters = new Map(library.map((a) => [a.entry.name, a.letter]));
    expect(letters.get("Édith Piaf")).toBe("E");
    expect(letters.get("Röyksopp")).toBe("R");
    expect(letters.get("björk")).toBe("B");
    expect(letters.get("2Pac")).toBe("#");
    expect(letters.get("坂本龍一")).toBe("#");
  });
});

describe("filterArtists", () => {
  it("keeps every artist for an empty or blank query", () => {
    expect(filterArtists(library, "")).toHaveLength(library.length);
    expect(filterArtists(library, "   ")).toHaveLength(library.length);
  });

  it("matches whatever the case", () => {
    expect(names(filterArtists(library, "RADIO"))).toEqual(["Radiohead"]);
    expect(names(filterArtists(library, "bjork"))).toEqual(["björk"]);
  });

  it("matches with or without the accents the name has or the query types", () => {
    expect(names(filterArtists(library, "edith"))).toEqual(["Édith Piaf"]);
    expect(names(filterArtists(library, "Édith"))).toEqual(["Édith Piaf"]);
    expect(names(filterArtists(library, "sigur ros"))).toEqual(["Sigur Rós"]);
    expect(names(filterArtists(library, "rós"))).toContain("Sigur Rós");
    expect(names(filterArtists(library, "motley crue"))).toEqual(["Mötley Crüe"]);
  });

  it("ranks names that start with the query, then words that do, then the rest", () => {
    const list = indexArtists(
      ["Sunset Rubdown", "Brian Eno", "Enobarbus", "Eno", "Beno"].map((n) => entry(n)),
    );
    expect(names(filterArtists(list, "eno"))).toEqual(["Eno", "Enobarbus", "Brian Eno", "Beno"]);
  });

  it("finds a name without its spaces and punctuation", () => {
    expect(names(filterArtists(library, "acdc"))).toEqual(["AC/DC"]);
    expect(names(filterArtists(library, "sigurros"))).toEqual(["Sigur Rós"]);
  });

  it("finds nothing for a name the library does not hold", () => {
    expect(filterArtists(library, "zzzz")).toEqual([]);
  });

  it("finds non-Latin names", () => {
    expect(names(filterArtists(library, "坂本"))).toEqual(["坂本龍一"]);
  });
});

describe("listRows", () => {
  it("heads each letter once, in order", () => {
    const rows = listRows(library, true);
    const heads = rows.flatMap((r) => (r.kind === "letter" ? [r.letter] : []));
    expect(heads).toEqual(["A", "B", "E", "M", "R", "S", "T", "Z", "#"]);
    expect(rows.filter((r) => r.kind === "artist")).toHaveLength(library.length);
  });

  it("is flat while filtering, each artist with its place in the results", () => {
    const rows = listRows(filterArtists(library, "r"), false);
    expect(rows.every((r) => r.kind === "artist")).toBe(true);
    expect(rows.map((r) => (r.kind === "artist" ? r.position : -1))).toEqual(rows.map((_, i) => i));
  });
});
