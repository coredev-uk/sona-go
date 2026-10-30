import { describe, expect, it } from "vitest";

import { keys } from "@/shared/lib/keys";
import { cachedKeys as cached, staleAfter } from "@/shared/testing/query-cache";

import { syncDoneInvalidations, syncInvalidations } from "./invalidations";

describe("syncInvalidations", () => {
  it("ignores progress inside a domain", () => {
    expect(syncInvalidations({ domain: "songs", done: 100, total: 900, finished: false })).toEqual(
      [],
    );
  });

  it("refreshes a finished domain's lists and the counts", () => {
    const stale = staleAfter(
      syncInvalidations({ domain: "albums", done: 120, total: 120, finished: true }),
      cached,
    );
    expect(stale).toEqual([
      "library/counts",
      "library/sync-status",
      "library/albums/dateAdded/true/2",
      "library/album/l.abc",
    ]);
  });

  it("refreshes playlist contents when playlist tracks finish", () => {
    const stale = staleAfter(
      syncInvalidations({ domain: "playlist-tracks", done: 3, total: 3, finished: true }),
      cached,
    );
    expect(stale).toContain("library/playlist/p.abc");
    expect(stale).not.toContain("library/playlists/title/false/0");
  });

  it("refreshes favourites and item states when ratings finish, and Recently Added with albums", () => {
    const withViews = [
      ...cached,
      [...keys.libraryFavorites(0)],
      [...keys.libraryRecentlyAdded(0)],
      [...keys.itemState("songs", "1440")],
    ];
    const ratings = staleAfter(
      syncInvalidations({ domain: "ratings", done: 9, total: 9, finished: true }),
      withViews,
    );
    expect(ratings).toContain("library/favorite-songs/0");
    expect(ratings).toContain("library/state/songs/1440");
    expect(ratings).not.toContain("library/recently-added/0");
    const albums = staleAfter(
      syncInvalidations({ domain: "albums", done: 1, total: 1, finished: true }),
      withViews,
    );
    expect(albums).toContain("library/recently-added/0");
    expect(albums).not.toContain("library/favorite-songs/0");
  });

  it("refreshes the whole library on sync:done and nothing else", () => {
    const stale = staleAfter(syncDoneInvalidations, cached);
    expect(stale.every((k) => k.startsWith("library/"))).toBe(true);
    expect(stale).toHaveLength(8);
  });
});
