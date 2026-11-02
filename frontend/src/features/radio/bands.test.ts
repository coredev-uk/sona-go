import type { Item, Section, Station } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { placeSections, sectionPlace } from "./bands";

const station = (kind: string): Item =>
  ({ kind: "station", id: kind, station: { kind } as Station }) as Item;
const item = (kind: string): Item => ({ kind, id: kind }) as Item;
const section = (id: string, items: Item[], kind = "shelf"): Section =>
  ({ id, kind, title: id, items }) as Section;

describe("radio bands", () => {
  it("places a section by its kind, else by most of its items", () => {
    expect(sectionPlace(section("Local", [station("broadcast")], "stations"))).toBe("live");
    expect(sectionPlace(section("On air", [station("live")], "live"))).toBe("live");
    expect(sectionPlace(section("More", [item("room")], "links"))).toBe("browse");
    expect(sectionPlace(section("Genres", [station("tracks"), station("tracks")]))).toBe(
      "stations",
    );
    expect(sectionPlace(section("Episodes", [station("episode"), item("album")]))).toBe("shows");
    expect(sectionPlace(section("Hosts", [item("curator")]))).toBe("shows");
    expect(sectionPlace(section("Interviews", [item("music-video")]))).toBe("shows");
    // Apple's featured row of live stations stays with the live stations.
    expect(sectionPlace(section("", [station("live"), station("live")], "hero"))).toBe("live");
    // A tie goes to the first item's place.
    expect(sectionPlace(section("Mixed", [item("grouping"), station("tracks")]))).toBe("browse");
  });

  it("orders the bands and keeps Apple's order inside each", () => {
    const sections = [
      section("Latest Episodes", [station("episode")]),
      section("Local Broadcasters", [station("broadcast")], "stations"),
      section("Top Stations", [station("tracks")]),
      section("All Shows", [item("curator")]),
      section("More to Explore", [item("room")], "links"),
      section("Stations by Genre", [station("tracks")]),
    ];
    const { live, bands } = placeSections(sections);
    expect(live.map((s) => s.id)).toEqual(["Local Broadcasters"]);
    expect(bands.map((b) => [b.id, b.sections.map((s) => s.id)])).toEqual([
      ["stations", ["Top Stations", "Stations by Genre"]],
      ["shows", ["Latest Episodes", "All Shows"]],
      ["browse", ["More to Explore"]],
    ]);
  });

  it("keeps every band, empty or not", () => {
    expect(placeSections([]).bands.map((b) => [b.id, b.sections.length])).toEqual([
      ["stations", 0],
      ["shows", 0],
      ["browse", 0],
    ]);
  });

  it("keeps a paragraph with the section it introduces", () => {
    const intro = section("intro", [], "text");
    const outro = section("outro", [], "text");
    const { bands } = placeSections([
      section("Episodes", [station("episode")]),
      intro,
      section("Genres", [station("tracks")]),
      outro,
    ]);
    expect(bands[0].sections.map((s) => s.id)).toEqual(["intro", "Genres", "outro"]);
    expect(bands[1].sections.map((s) => s.id)).toEqual(["Episodes"]);
  });
});
