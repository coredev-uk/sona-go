import type { AlbumPage } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { LibraryAlbumsPage } from "./pages";
import { albumsSearch } from "./search-params";

const albums = vi.fn<(sort: string, desc: boolean, type: string) => Promise<AlbumPage>>();
const counts = vi.fn();
const navigate = vi.fn();
let search: Record<string, unknown> = {};

vi.mock("@bindings/services", () => ({
  LibraryService: {
    Albums: (sort: string, desc: boolean, type: string) => albums(sort, desc, type),
    Counts: () => counts(),
    SyncStatus: () => Promise.resolve({ running: false, lastSuccess: 1 }),
  },
  PlaybackService: {},
}));
vi.mock("@/features/player", () => ({ replaceQueue: () => {} }));
vi.mock("@/features/auth", () => ({
  useAuth: () => ({ data: { state: "signed_in" } }),
  isSignedOut: () => false,
}));
vi.mock("@/shared/actions/ActionMenu", () => ({ ActionMenu: () => null }));
vi.mock("@/shared/components/CoverGrid", () => ({
  CoverGrid: ({ items }: { items: ({ key: string; title: string } | undefined)[] }) => (
    <ul>
      {items.map((it, i) => (
        <li key={it?.key ?? i}>{it?.title}</li>
      ))}
    </ul>
  ),
}));
vi.mock("@tanstack/react-router", () => ({
  getRouteApi: () => ({
    useSearch: () => albumsSearch.parse(search),
    useNavigate: () => navigate,
  }),
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}));

const page = (names: string[], total = names.length): AlbumPage =>
  ({
    items: names.map((name) => ({ id: name, name, artistName: "A", art: {} })),
    total,
  }) as AlbumPage;

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <LibraryAlbumsPage />
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
}

beforeEach(() => {
  search = {};
  counts.mockResolvedValue({ songs: 0, albums: 4, artists: 0, playlists: 0 });
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("albumsSearch", () => {
  it("keeps a known type and drops an unknown one", () => {
    expect(albumsSearch.parse({ type: "singles", sort: "title" })).toMatchObject({
      type: "singles",
      sort: "title",
    });
    expect(albumsSearch.parse({ type: "box-sets" }).type).toBeUndefined();
  });
});

describe("LibraryAlbumsPage type filter", () => {
  it("lists every album until a type is chosen, then asks Go for that type", async () => {
    albums.mockResolvedValue(page(["Dawn - Single", "Cargo"]));
    renderPage();
    expect(await screen.findByText("Cargo")).toBeTruthy();
    expect(albums).toHaveBeenLastCalledWith("dateAdded", true, "");
    expect(screen.getByRole("button", { name: "All" }).getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Singles & EPs" }));
    const arg = navigate.mock.calls[0]?.[0] as { search: (p: object) => object };
    // The sort already chosen stays when the type changes.
    expect(arg.search({ sort: "title", desc: false })).toEqual({
      sort: "title",
      desc: false,
      type: "singles",
    });
  });

  it("reads the type from the URL, with the sort", async () => {
    search = { type: "singles", sort: "artist", desc: false };
    albums.mockResolvedValue(page(["Dawn - Single"]));
    renderPage();
    expect(await screen.findByText("Dawn - Single")).toBeTruthy();
    expect(albums).toHaveBeenLastCalledWith("artist", false, "singles");
    expect(screen.getByRole("button", { name: "Singles & EPs" }).getAttribute("aria-pressed")).toBe(
      "true",
    );
    // The header says how many, and the result is announced.
    expect(screen.getAllByText("1 single or EP")).toHaveLength(2);
    expect(screen.getByRole("status").textContent).toBe("1 single or EP");
  });

  it("says so, and offers all albums, when the type has none but the library has albums", async () => {
    search = { type: "singles" };
    albums.mockResolvedValue(page([]));
    renderPage();
    expect(
      await screen.findByRole("heading", { name: "No singles or EPs in your library" }),
    ).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe("No singles or EPs in your library");
    fireEvent.click(screen.getByRole("button", { name: "Show all albums" }));
    const arg = navigate.mock.calls[0]?.[0] as { search: (p: object) => object };
    expect(arg.search({ type: "singles" })).toEqual({ type: undefined });
  });
});
