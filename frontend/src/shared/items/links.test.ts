import type { Item, Station } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { itemLink, itemShape, itemSubtitle, roomLink } from "./links";

const item = (over: Partial<Item>): Item =>
  ({
    kind: "album",
    id: "1",
    name: "N",
    subtitle: "",
    caption: "",
    art: { small: "", medium: "", large: "" },
    wide: { small: "", medium: "", large: "" },
    color: "",
    explicit: false,
    url: "",
    ...over,
  }) as Item;

describe("itemLink", () => {
  it("opens each kind's page", () => {
    expect(itemLink(item({ kind: "album", id: "9" }))).toEqual({
      to: "/album/$id",
      params: { id: "9" },
    });
    expect(itemLink(item({ kind: "curator", id: "99" }))).toEqual({
      to: "/curator/$id",
      params: { id: "99" },
    });
    expect(itemLink(item({ kind: "multiroom", id: "5" }))?.to).toBe("/multiroom/$id");
    expect(itemLink(item({ kind: "station", id: "ra.1" }))?.to).toBe("/station/$id");
  });
  it("opens a station's page only when it has one", () => {
    const station = (hasPage: boolean) => ({ kind: "broadcast", hasPage }) as Station;
    expect(itemLink(item({ kind: "station", id: "ra.1", station: station(true) }))?.to).toBe(
      "/station/$id",
    );
    // A local broadcaster has no schedule: it plays instead of opening.
    expect(
      itemLink(item({ kind: "station", id: "ra.1", station: station(false) })),
    ).toBeUndefined();
  });
  it("sends charts links to the genre's charts", () => {
    expect(itemLink(item({ kind: "charts", id: "" }))).toEqual({ to: "/charts", search: {} });
    expect(itemLink(item({ kind: "charts", id: "14" }))).toEqual({
      to: "/charts",
      search: { genre: "14" },
    });
  });
  it("opens Apple's concerts links in the Concerts page, not the browser", () => {
    expect(
      itemLink(item({ kind: "concerts", id: "", url: "https://music.apple.com/gb/concerts" })),
    ).toEqual({
      to: "/concerts",
    });
  });
  it("has no page for web-only items", () => {
    expect(
      itemLink(item({ kind: "link", url: "https://music.apple.com/gb/post/123" })),
    ).toBeUndefined();
  });
});

describe("item presentation", () => {
  it("joins the reason and the artist line", () => {
    expect(itemSubtitle(item({ caption: "New release", subtitle: "Radiohead" }))).toBe(
      "New release · Radiohead",
    );
    expect(itemSubtitle(item({ subtitle: "Radiohead" }))).toBe("Radiohead");
  });
  it("draws artists round", () => {
    expect(itemShape(item({ kind: "artist" }))).toBe("round");
    expect(itemShape(item({ kind: "playlist" }))).toBe("square");
  });
  it("links a section to its room only when it has one", () => {
    const s = {
      id: "s",
      kind: "shelf",
      title: "T",
      text: "",
      items: [],
      room: "",
      seeAll: false,
      source: "",
    };
    expect(roomLink(s)).toBeUndefined();
    expect(roomLink({ ...s, room: "77" })).toEqual({ to: "/room/$id", params: { id: "77" } });
  });
});
