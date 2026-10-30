import { describe, expect, it } from "vitest";

import { moveActive, splitMatch } from "./combobox";
import { groupTitle, hasSeeAll, kindLabel } from "./groups";

describe("moveActive", () => {
  it("enters the list from the field at either end", () => {
    expect(moveActive(-1, 1, 4)).toBe(0);
    expect(moveActive(-1, -1, 4)).toBe(3);
  });
  it("moves within the list and returns to the field past the ends", () => {
    expect(moveActive(1, 1, 4)).toBe(2);
    expect(moveActive(3, 1, 4)).toBe(-1);
    expect(moveActive(0, -1, 4)).toBe(-1);
  });
  it("stays in the field when there is nothing to move to", () => {
    expect(moveActive(-1, 1, 0)).toBe(-1);
    expect(moveActive(2, -1, 0)).toBe(-1);
  });
});

describe("splitMatch", () => {
  it("finds what was typed, ignoring case and outer spaces", () => {
    expect(splitMatch("taylor swift", " Tay")).toEqual(["", "tay", "lor swift"]);
    expect(splitMatch("the taylor show", "taylor")).toEqual(["the ", "taylor", " show"]);
  });
  it("returns the whole text as the remainder when nothing matches", () => {
    expect(splitMatch("blue", "red")).toEqual(["", "", "blue"]);
    expect(splitMatch("blue", "  ")).toEqual(["", "", "blue"]);
  });
});

describe("groups", () => {
  it("titles known and unknown groups in sentence case", () => {
    expect(groupTitle("radio_episode")).toBe("Radio episodes");
    expect(groupTitle("shared_playlist")).toBe("Playlists by listeners");
    expect(groupTitle("video_clip")).toBe("Video clips");
  });
  it("offers see-all for paged or truncated groups, never for top results", () => {
    expect(hasSeeAll("song", 21, 8, 21)).toBe(true);
    expect(hasSeeAll("song", 0, 8, 12)).toBe(true);
    expect(hasSeeAll("song", 0, 8, 8)).toBe(false);
    expect(hasSeeAll("top", 21, 8, 21)).toBe(false);
  });
  it("labels station kinds", () => {
    expect(kindLabel("station", { kind: "episode" })).toBe("Radio episode");
    expect(kindLabel("station", { kind: "live" })).toBe("Live radio");
    expect(kindLabel("station", { kind: "tracks" })).toBe("Station");
    expect(kindLabel("music-video")).toBe("Music video");
  });
});
