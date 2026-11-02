import { ShelfKind, type Album, type ArtistShelf, type CatalogArtist } from "@bindings/services";
import { describe, expect, it } from "vitest";

import {
  artistLine,
  bioLead,
  catalogueColumns,
  layoutFor,
  releaseCount,
  releaseTitle,
  shelvesFor,
} from "./artist";

const album = (id: string) => ({ id, name: id }) as Album;

function shelf(view: string, ids: string[], more = false): ArtistShelf {
  return {
    view,
    kind: ShelfKind.ShelfAlbums,
    more,
    albums: ids.map(album),
    playlists: [],
    videos: [],
    artists: [],
  };
}

function page(shelves: ArtistShelf[], latest?: string): CatalogArtist {
  return { shelves, latestRelease: latest ? album(latest) : null } as unknown as CatalogArtist;
}

describe("releaseTitle", () => {
  it("moves Apple's single and EP suffix out of the title", () => {
    expect(releaseTitle("Daydreaming - Single")).toEqual({ title: "Daydreaming", kind: "single" });
    expect(releaseTitle("Com Lag: 2+2=5 - EP")).toEqual({ title: "Com Lag: 2+2=5", kind: "ep" });
  });

  it("leaves other titles whole, dashes included", () => {
    expect(releaseTitle("The King of Limbs - Live from the Basement")).toEqual({
      title: "The King of Limbs - Live from the Basement",
    });
    expect(releaseTitle("Single")).toEqual({ title: "Single" });
  });
});

describe("releaseCount", () => {
  it("counts each release once when the page holds them all", () => {
    const p = page(
      [
        shelf("full-albums", ["a", "b"]),
        shelf("singles", ["s"]),
        shelf("featured-albums", ["a"]),
        shelf("appears-on-albums", ["x", "y"]),
      ],
      "s",
    );
    expect(releaseCount(p)).toBe(3);
  });

  it("is unknown while any of the artist's own release lists has more to fetch", () => {
    expect(releaseCount(page([shelf("full-albums", ["a"]), shelf("singles", ["s"], true)]))).toBe(
      undefined,
    );
    // Others' albums with more don't make the artist's own count unknown.
    expect(
      releaseCount(page([shelf("singles", ["s"]), shelf("appears-on-albums", ["x"], true)])),
    ).toBe(1);
  });
});

describe("shelvesFor", () => {
  it("returns the views' shelves in the order asked, not Apple's", () => {
    const p = page([shelf("compilation-albums", ["c"]), shelf("singles", ["s"])]);
    expect(
      shelvesFor(p, ["singles", "live-albums", "compilation-albums"]).map((s) => s.view),
    ).toEqual(["singles", "compilation-albums"]);
  });
});

describe("artistLine", () => {
  it("is the catalogue line: kind, genre, origin, then the count when known", () => {
    expect(artistLine("Alternative", "Abingdon, Oxfordshire, England")).toBe(
      "Artist · Alternative · Abingdon, Oxfordshire, England",
    );
    expect(artistLine("", "", 1)).toBe("Artist · 1 release");
    expect(artistLine("Pop", "", undefined, "2 albums, 9 songs in your library")).toBe(
      "Artist · Pop · 2 albums, 9 songs in your library",
    );
  });
});

describe("bioLead", () => {
  it("takes the first paragraph", () => {
    expect(bioLead("First line.\nStill first.\n\nSecond.")).toBe("First line.\nStill first.");
    expect(bioLead("")).toBe("");
  });
});

describe("layout", () => {
  it("adds the second column from 900px and drops the portrait under 720px", () => {
    expect(layoutFor(1144)).toMatchObject({ wide: true, portrait: 200, slip: 112 });
    expect(layoutFor(800)).toMatchObject({ wide: false, portrait: 200, slip: 56 });
    expect(layoutFor(660)).toMatchObject({ wide: false, portrait: 136 });
    // Before the first layout the page is measured as 0: the full-size portrait, no jump.
    expect(layoutFor(0).portrait).toBe(200);
  });

  it("sets catalogue lists side by side as far as they fit, one column each", () => {
    expect(catalogueColumns(1144, 3)).toBe(3);
    expect(catalogueColumns(660, 3)).toBe(2);
    expect(catalogueColumns(200, 3)).toBe(1);
    expect(catalogueColumns(1144, 1)).toBe(1);
  });
});
