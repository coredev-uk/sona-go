import type { Item, Section } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { planHome, withoutSharedCaption } from "./plan";

const item = (kind: string, caption = "") => ({ kind, id: kind, caption }) as Item;

function shelf(id: string, kind = "shelf", items = [item("album")], source = "") {
  return { id, kind, title: id, text: "", items, room: "", seeAll: false, source } as Section;
}

describe("planHome", () => {
  it("leads with the first feature shelf and lists plain shelves past the first few", () => {
    const sections = [
      shelf("top", "feature"),
      shelf("recent", "shelf", [item("album")], "recently-played"),
      shelf("oasis"),
      shelf("made", "feature"),
      shelf("zara"),
      shelf("stations", "shelf", [item("station"), item("station")]),
      shelf("new"),
      shelf("hero", "hero"),
      shelf("country"),
      shelf("pop"),
      shelf("replay", "feature"),
    ];
    const plan = planHome(sections);
    expect(plan.lead?.id).toBe("top");
    // Recently Played, stations, features and heroes never move to the index.
    expect(plan.open.map((s) => s.id)).toEqual([
      "recent",
      "oasis",
      "made",
      "zara",
      "stations",
      "new",
      "hero",
      "replay",
    ]);
    expect(plan.index.map((s) => s.id)).toEqual(["country", "pop"]);
  });

  it("keeps the concerts shelf, which has no items of its own, in the open", () => {
    const sections = [
      shelf("top", "feature"),
      shelf("a"),
      shelf("b"),
      shelf("c"),
      shelf("d"),
      shelf("concerts", "concerts", []),
    ];
    const plan = planHome(sections);
    expect(plan.open.map((s) => s.id)).toEqual(["a", "b", "c", "concerts"]);
    expect(plan.index.map((s) => s.id)).toEqual(["d"]);
  });

  it("has no lead without a feature shelf with items", () => {
    const plan = planHome([shelf("top", "feature", []), shelf("oasis")]);
    expect(plan.lead).toBeUndefined();
    expect(plan.open.map((s) => s.id)).toEqual(["top", "oasis"]);
  });
});

describe("withoutSharedCaption", () => {
  it("drops a caption every item shares and keeps varied ones", () => {
    const same = [item("playlist", "Apple Music for You"), item("playlist", "Apple Music for You")];
    expect(withoutSharedCaption(same).map((i) => i.caption)).toEqual(["", ""]);
    const varied = [item("album", "New Release"), item("station", "Made for You")];
    expect(withoutSharedCaption(varied)).toBe(varied);
  });
});
