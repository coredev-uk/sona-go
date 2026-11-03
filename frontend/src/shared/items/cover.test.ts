import type { Item, Station } from "@bindings/services";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { openWithoutPage, toCoverItem } from "./cover";

const h = vi.hoisted(() => ({ played: vi.fn(), open: vi.fn() }));
vi.mock("./play", () => ({
  canPlay: (i: Item) => !!i.station?.playable,
  playItem: h.played,
  canPlayVideo: () => false,
}));
vi.mock("@wailsio/runtime", () => ({ Browser: { OpenURL: h.open } }));

const station = (hasPage: boolean): Item =>
  ({
    kind: "station",
    id: "ra.1",
    name: "Local FM",
    subtitle: "",
    caption: "",
    url: "https://music.apple.com/gb/station/local-fm/ra.1",
    art: { small: "", medium: "", large: "" },
    wide: { small: "", medium: "", large: "" },
    station: { kind: hasPage ? "live" : "broadcast", hasPage, playable: true } as Station,
  }) as Item;

beforeEach(() => {
  h.played.mockClear();
  h.open.mockClear();
});

describe("a station as a tile", () => {
  it("opens its page and has a play control when it has a page", () => {
    const tile = toCoverItem(station(true), 0);
    expect(tile.link?.to).toBe("/station/$id");
    expect(tile.play?.label).toBe("Local FM");
    expect(tile.playsOnOpen).toBeUndefined();
  });

  it("plays when opened, with no page and no second play control, when it has none", () => {
    const tile = toCoverItem(station(false), 0);
    expect(tile.link).toBeUndefined();
    expect(tile.play).toBeUndefined();
    expect(tile.playsOnOpen).toBe(true);
    tile.onOpen?.();
    expect(h.played).toHaveBeenCalledTimes(1);
    expect(h.open).not.toHaveBeenCalled();
  });
});

describe("openWithoutPage", () => {
  it("plays a station and opens a web-only item in the browser", () => {
    openWithoutPage(station(false));
    expect(h.played).toHaveBeenCalledTimes(1);
    const web = { kind: "link", id: "", name: "Concert", url: "https://example.com/c" } as Item;
    openWithoutPage(web);
    expect(h.open).toHaveBeenCalledWith("https://example.com/c");
    expect(h.played).toHaveBeenCalledTimes(1);
  });
});
