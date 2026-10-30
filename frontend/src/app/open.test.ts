import { describe, expect, it } from "vitest";

import { routeForOpen } from "./open";

describe("routeForOpen", () => {
  it("maps each link kind to its page", () => {
    expect(routeForOpen({ kind: "album", id: "1097861387" })).toEqual({
      to: "/album/$id",
      params: { id: "1097861387" },
      search: {},
    });
    expect(routeForOpen({ kind: "library-album", id: "l.abc" })).toMatchObject({
      params: { id: "l.abc" },
    });
    expect(routeForOpen({ kind: "artist", id: "657515" })).toMatchObject({ to: "/artist/$id" });
    expect(routeForOpen({ kind: "playlist", id: "pl.u-123" })).toMatchObject({
      to: "/playlist/$id",
    });
    expect(routeForOpen({ kind: "library-playlist", id: "p.abc" })).toMatchObject({
      to: "/playlist/$id",
    });
    expect(routeForOpen({ kind: "music-video", id: "1442907386" })).toMatchObject({
      to: "/music-video/$id",
    });
    expect(routeForOpen({ kind: "station", id: "ra.978194965" })).toMatchObject({
      to: "/station/$id",
      params: { id: "ra.978194965" },
    });
    expect(routeForOpen({ kind: "curator", id: "976439548" })).toMatchObject({
      to: "/curator/$id",
    });
  });

  it("opens a song in its album, or on its own page", () => {
    expect(routeForOpen({ kind: "song", id: "1097862703", albumId: "1097861387" })).toEqual({
      to: "/album/$id",
      params: { id: "1097861387" },
      search: { song: "1097862703" },
    });
    expect(routeForOpen({ kind: "song", id: "1097862703" })).toEqual({
      to: "/song/$id",
      params: { id: "1097862703" },
    });
  });

  it("ignores unknown kinds and empty ids", () => {
    expect(routeForOpen({ kind: "room", id: "6449178938" })).toBeUndefined();
    expect(routeForOpen({ kind: "album", id: "" })).toBeUndefined();
  });
});
