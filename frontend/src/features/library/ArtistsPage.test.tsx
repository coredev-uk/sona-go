import type { Album, ArtistAlbums, ArtistEntry } from "@bindings/services";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { TooltipProvider } from "@/shared/ui/tooltip";

import { LibraryArtistsPage } from "./ArtistsPage";

const url = vi.hoisted(() => ({
  search: {} as { artist?: string },
  navigate: vi.fn(),
  width: 1144,
}));

vi.mock("@tanstack/react-router", () => ({
  getRouteApi: () => ({ useSearch: () => url.search, useNavigate: () => url.navigate }),
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="/" className={className}>
      {children}
    </a>
  ),
}));
vi.mock("@/features/artist", () => ({
  releaseTitle: (name: string) => {
    const m = name.match(/^(.+?) - (Single|EP)$/);
    return m ? { title: m[1], kind: m[2] === "EP" ? "ep" : "single" } : { title: name };
  },
}));
vi.mock("@/features/player", () => ({ playSongs: vi.fn(), shuffleSongs: vi.fn() }));
vi.mock("@/shared/actions/ActionMenu", () => ({
  ActionMenu: () => null,
  ItemActionMenu: () => null,
}));
vi.mock("@/shared/components/CoverGrid", () => ({
  CoverGrid: ({ items, label }: { items: { key: string; title: string }[]; label: string }) => (
    <ul aria-label={label}>
      {items.map((i) => (
        <li key={i.key}>{i.title}</li>
      ))}
    </ul>
  ),
}));
vi.mock("@/shared/tracks/TrackList", () => ({
  TrackList: ({ songs, label }: { songs: { id: string; name: string }[]; label: string }) => (
    <ul aria-label={label}>
      {songs.map((s) => (
        <li key={s.id}>{s.name}</li>
      ))}
    </ul>
  ),
}));
vi.mock("@bindings/services", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@bindings/services")>()),
  LibraryService: {
    Artist: vi.fn(async (id: string) => ({
      songs: [
        { id: `${id}-s1`, name: "Padam" },
        { id: `${id}-s2`, name: "La Vie en rose" },
      ],
    })),
  },
}));
vi.mock("./gate", () => ({ useLibraryGate: () => null }));

const entry = (name: string, albums: number, songs: number): ArtistEntry =>
  ({ id: `r.${name}`, catalogId: `c.${name}`, name, art: "", albums, songs }) as ArtistEntry;

const album = (id: string, name: string): Album =>
  ({ id, name, releaseDate: "2020-01-01", art: { small: "", medium: "", large: "" } }) as Album;

const entries = [
  entry("Sigur Rós", 2, 20),
  entry("Björk", 3, 30),
  entry("Beyoncé", 1, 12),
  entry("Radiohead", 4, 44),
  entry("Édith Piaf", 0, 3),
];

vi.mock("./queries", async () => {
  const { indexArtists: index } = await import("./artists");
  return {
    useArtistIndex: () => ({
      data: index(entries),
      isPending: false,
      error: null,
      refetch: vi.fn(),
      isRefetching: false,
    }),
    useArtistAlbums: (id: string | undefined) => ({
      data: id
        ? ({
            artist: { id, name: id.slice(2) },
            // Édith Piaf has songs in the library, but no album.
            albums: id.includes("Piaf")
              ? []
              : [
                  album(`${id}-1`, `First of ${id.slice(2)}`),
                  { ...album(`${id}-2`, "Hush - EP"), single: true },
                ],
          } as unknown as ArtistAlbums)
        : undefined,
      isPending: false,
      error: null,
      refetch: vi.fn(),
      isRefetching: false,
    }),
  };
});

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
  Object.defineProperty(HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get: () => url.width,
  });
  // A 600px tall window: the virtualizer renders the rows that fit.
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get: () => 600,
  });
  Element.prototype.scrollTo = vi.fn();
});
beforeEach(() => {
  url.search = {};
  url.width = 1144;
  url.navigate = vi.fn();
});
afterEach(cleanup);

function Providers({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      <I18nRoot>
        <TooltipProvider>{children}</TooltipProvider>
      </I18nRoot>
    </QueryClientProvider>
  );
}
const renderPage = () => render(<LibraryArtistsPage />, { wrapper: Providers });
const filter = () => screen.getByRole("combobox", { name: "Filter artists" });
const options = () => within(screen.getByRole("listbox")).getAllByRole("option");
const optionNames = () => options().map((o) => o.textContent?.replace(/\d.*$/, ""));
const detail = () => document.querySelector<HTMLElement>("[data-detail]");

describe("LibraryArtistsPage beside the detail", () => {
  it("lists every artist A to Z and opens the first when the URL names none", () => {
    renderPage();
    expect(optionNames()).toEqual(["Beyoncé", "Björk", "Édith Piaf", "Radiohead", "Sigur Rós"]);
    expect(detail()?.dataset.detail).toBe("r.Beyoncé");
    expect(within(detail()!).getByRole("heading", { name: "Beyoncé" })).toBeTruthy();
    // Nothing was written to the URL for the default.
    expect(url.navigate).not.toHaveBeenCalled();
  });

  it("opens the artist the URL names, with their library albums and singles apart", () => {
    url.search = { artist: "r.Radiohead" };
    renderPage();
    expect(detail()?.dataset.detail).toBe("r.Radiohead");
    expect(screen.getByRole("option", { name: /Radiohead/ }).getAttribute("aria-selected")).toBe(
      "true",
    );
    const albums = screen.getByRole("list", { name: "Albums by Radiohead in your library" });
    expect(within(albums).getByText("First of Radiohead")).toBeTruthy();
    const singles = screen.getByRole("list", {
      name: "Singles and EPs by Radiohead in your library",
    });
    // Apple's " - EP" suffix is the kind, not part of the title.
    expect(within(singles).getByText("Hush")).toBeTruthy();
  });

  it("lists the songs of an artist with songs but no album, where the albums would be", async () => {
    url.search = { artist: "r.Édith Piaf" };
    renderPage();
    const songs = await screen.findByRole("list", { name: "Songs by Édith Piaf in your library" });
    expect(within(songs).getByText("La Vie en rose")).toBeTruthy();
    expect(screen.queryByRole("list", { name: /^Albums by/ })).toBeNull();
  });

  it("opens the first artist for a URL naming one the library doesn't hold", () => {
    url.search = { artist: "r.Nobody" };
    renderPage();
    expect(detail()?.dataset.detail).toBe("r.Beyoncé");
  });

  it("writes a clicked artist to the URL as a new history entry", () => {
    renderPage();
    fireEvent.click(screen.getByRole("option", { name: /Björk/ }));
    expect(url.navigate).toHaveBeenCalledWith({ search: { artist: "r.Björk" }, replace: false });
  });

  it("moves with ↑ and ↓ in the filter, replacing the history entry", () => {
    url.search = { artist: "r.Björk" };
    renderPage();
    fireEvent.keyDown(filter(), { key: "ArrowDown" });
    expect(url.navigate).toHaveBeenLastCalledWith({
      search: { artist: "r.Édith Piaf" },
      replace: true,
    });
    fireEvent.keyDown(filter(), { key: "ArrowUp" });
    expect(url.navigate).toHaveBeenLastCalledWith({
      search: { artist: "r.Beyoncé" },
      replace: true,
    });
  });

  it("stops at the ends of the list", () => {
    url.search = { artist: "r.Sigur Rós" };
    renderPage();
    fireEvent.keyDown(filter(), { key: "ArrowDown" });
    expect(url.navigate).toHaveBeenLastCalledWith({
      search: { artist: "r.Sigur Rós" },
      replace: true,
    });
  });

  it("filters without regard to case or accents, and shows the first match", () => {
    url.search = { artist: "r.Sigur Rós" };
    renderPage();
    fireEvent.change(filter(), { target: { value: "BJORK" } });
    expect(optionNames()).toEqual(["Björk"]);
    // The count says how many of the artists match.
    expect(screen.getByText("1 of 5 artists")).toBeTruthy();
    expect(detail()?.dataset.detail).toBe("r.Björk");
    // Typing doesn't write the URL; choosing does.
    expect(url.navigate).not.toHaveBeenCalled();
    fireEvent.change(filter(), { target: { value: "edith" } });
    expect(optionNames()).toEqual(["Édith Piaf"]);
  });

  it("says when no artist matches, and names the filter's result for assistive technology", () => {
    renderPage();
    fireEvent.change(filter(), { target: { value: "zzz" } });
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(screen.getByRole("status").textContent).toContain("No artist matches “zzz”");
    fireEvent.change(filter(), { target: { value: "r" } });
    expect(screen.getByRole("status").textContent).toMatch(/artists match/);
  });

  it("Enter opens the filter's top match in the URL", () => {
    url.search = { artist: "r.Sigur Rós" };
    renderPage();
    fireEvent.change(filter(), { target: { value: "rad" } });
    fireEvent.keyDown(filter(), { key: "Enter" });
    expect(url.navigate).toHaveBeenCalledWith({ search: { artist: "r.Radiohead" }, replace: true });
  });

  it("Escape clears the filter", () => {
    renderPage();
    fireEvent.change(filter(), { target: { value: "bj" } });
    fireEvent.keyDown(filter(), { key: "Escape" });
    expect((filter() as HTMLInputElement).value).toBe("");
    expect(options()).toHaveLength(5);
  });

  it("→ at the end of the filter goes into the detail, but moves the caret elsewhere", () => {
    renderPage();
    fireEvent.change(filter(), { target: { value: "rad" } });
    const input = filter() as HTMLInputElement;
    input.setSelectionRange(1, 1);
    fireEvent.keyDown(input, { key: "ArrowRight" });
    expect(url.navigate).not.toHaveBeenCalled();
    input.setSelectionRange(3, 3);
    fireEvent.keyDown(input, { key: "ArrowRight" });
    expect(url.navigate).toHaveBeenCalledWith({ search: { artist: "r.Radiohead" }, replace: true });
  });

  it("types on the list into the filter", () => {
    renderPage();
    fireEvent.keyDown(screen.getByRole("listbox"), { key: "s" });
    expect((filter() as HTMLInputElement).value).toBe("s");
  });

  it("marks the open artist as the listbox's active descendant", () => {
    url.search = { artist: "r.Björk" };
    renderPage();
    const id = screen.getByRole("option", { name: /Björk/ }).id;
    expect(filter().getAttribute("aria-activedescendant")).toBe(id);
    expect(screen.getByRole("listbox").getAttribute("aria-activedescendant")).toBe(id);
  });

  it("counts what the library holds of each artist", () => {
    renderPage();
    expect(screen.getByRole("option", { name: /Björk/ }).textContent).toContain(
      "3 albums, 30 songs",
    );
    expect(screen.getByRole("option", { name: /Édith Piaf/ }).textContent).toContain("3 songs");
  });
});

describe("LibraryArtistsPage in a narrow window", () => {
  beforeEach(() => {
    url.width = 708;
  });

  it("shows the list alone until an artist is chosen", () => {
    renderPage();
    expect(detail()).toBeNull();
    expect(options()).toHaveLength(5);
  });

  it("opens an artist from the list as a new history entry", () => {
    renderPage();
    fireEvent.click(screen.getByRole("option", { name: /Radiohead/ }));
    expect(url.navigate).toHaveBeenCalledWith({
      search: { artist: "r.Radiohead" },
      replace: false,
    });
  });

  it("moves a cursor with the arrows and opens it with Enter", () => {
    renderPage();
    fireEvent.keyDown(filter(), { key: "ArrowDown" });
    expect(url.navigate).not.toHaveBeenCalled();
    fireEvent.keyDown(filter(), { key: "ArrowDown" });
    fireEvent.keyDown(filter(), { key: "Enter" });
    expect(url.navigate).toHaveBeenCalledWith({ search: { artist: "r.Björk" }, replace: false });
  });

  it("opens the filter's top match with Enter", () => {
    renderPage();
    fireEvent.change(filter(), { target: { value: "sigur" } });
    fireEvent.keyDown(filter(), { key: "Enter" });
    expect(url.navigate).toHaveBeenCalledWith({
      search: { artist: "r.Sigur Rós" },
      replace: false,
    });
  });

  it("shows the artist the URL names, with a way back to the list", () => {
    url.search = { artist: "r.Björk" };
    const { container } = renderPage();
    expect(detail()?.dataset.detail).toBe("r.Björk");
    // The list's h1 is hidden, so the artist is the page's heading.
    expect(screen.getByRole("heading", { level: 1, name: "Björk" })).toBeTruthy();
    // The list stays mounted but hidden, so its scroll position and filter survive.
    expect(container.querySelector("#artist-list")?.closest(".hidden")).not.toBeNull();
    // And it marks the artist that is open, so Back shows where you were.
    expect(
      within(container.querySelector<HTMLElement>("#artist-list")!)
        .getByRole("option", { name: /Björk/, hidden: true })
        .getAttribute("aria-selected"),
    ).toBe("true");
    const back = screen.getByRole("button", { name: "Artists" });
    // Reached by reload: Back leaves the entry instead of pushing another.
    fireEvent.click(back);
    expect(url.navigate).toHaveBeenCalledWith({ search: {}, replace: true });
  });

  it("goes back through history after opening from the list", () => {
    const back = vi.spyOn(window.history, "back").mockImplementation(() => {});
    const { rerender } = renderPage();
    fireEvent.click(screen.getByRole("option", { name: /Björk/ }));
    url.search = { artist: "r.Björk" };
    act(() => rerender(<LibraryArtistsPage />));
    // Focus follows the view to its way back.
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Artists" }));
    fireEvent.click(screen.getByRole("button", { name: "Artists" }));
    expect(back).toHaveBeenCalled();
    // The browser went back: the list returns with focus in its filter.
    url.search = {};
    act(() => rerender(<LibraryArtistsPage />));
    expect(document.activeElement).toBe(filter());
    back.mockRestore();
  });

  it("falls back to the list for an artist the library doesn't hold", () => {
    url.search = { artist: "r.Nobody" };
    renderPage();
    expect(detail()).toBeNull();
  });
});
