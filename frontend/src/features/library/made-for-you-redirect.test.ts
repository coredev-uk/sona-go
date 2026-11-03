import { isRedirect } from "@tanstack/react-router";
import { describe, expect, it, vi } from "vitest";

import { libraryModule } from "./index";

vi.mock("./pages", () => ({
  LibraryAlbumsPage: () => null,
  LibraryArtistsPage: () => null,
  LibraryPlaylistsPage: () => null,
  LibrarySongsPage: () => null,
}));
vi.mock("./views", () => ({
  FavoriteSongsPage: () => null,
  MusicVideosPage: () => null,
  RecentlyAddedPage: () => null,
}));
vi.mock("./HistoryPage", () => ({ HistoryPage: () => null }));

describe("the retired Made for you page", () => {
  const route = libraryModule.routes.find(
    (r) => (r.options as { path?: string }).path === "/library/made-for-you",
  );

  it("opens Home for an old link or a saved last page", () => {
    const beforeLoad = route?.options.beforeLoad as (() => unknown) | undefined;
    let thrown: unknown;
    try {
      beforeLoad?.();
    } catch (e) {
      thrown = e;
    }
    expect(isRedirect(thrown)).toBe(true);
    expect((thrown as { options: { to: string; replace: boolean } }).options).toMatchObject({
      to: "/",
      replace: true,
    });
  });

  it("is no longer in the sidebar", () => {
    expect(libraryModule.nav?.map((n) => n.to)).not.toContain("/library/made-for-you");
  });
});
