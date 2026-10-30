import { describe, expect, it } from "vitest";

import { matchScore, rankCommands } from "./rank";

const item = (label: string, keywords?: string) => ({
  label: () => label,
  keywords: keywords === undefined ? undefined : () => keywords,
});

const items = [
  item("Show or hide the queue", "up next"),
  item("Next track", "skip forward"),
  item("Library · Albums", "library"),
  item("Albums of the year"),
  item("Play or pause", "resume stop"),
  item("Café playlists"),
];

const labels = (q: string) => rankCommands(q, items).map((i) => i.label());

describe("rankCommands", () => {
  it("puts a label that starts with the query before one that only contains it", () => {
    expect(labels("next")).toEqual(["Next track", "Show or hide the queue"]);
  });

  it("ranks word starts above substrings and keywords last", () => {
    expect(labels("alb")).toEqual(["Albums of the year", "Library · Albums"]);
    expect(labels("skip")).toEqual(["Next track"]);
  });

  it("matches every typed word at word starts, in any order", () => {
    expect(labels("hide queue")).toEqual(["Show or hide the queue"]);
    expect(labels("pause play")).toEqual(["Play or pause"]);
  });

  it("folds case and accents", () => {
    expect(labels("CAFE")).toEqual(["Café playlists"]);
  });

  it("falls back to letters in order", () => {
    expect(matchScore("ntr", item("Next track"))).toBeGreaterThan(0);
    expect(matchScore("xyz", item("Next track"))).toBe(0);
  });

  it("keeps the given order for an empty query", () => {
    expect(labels("")).toEqual(items.map((i) => i.label()));
  });
});
