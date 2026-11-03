import { ShelfKind, type Album, type ArtistShelf, type CatalogArtist } from "@bindings/services";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { ArtistPage } from "./ArtistPage";

vi.mock("@tanstack/react-router", () => ({
  getRouteApi: () => ({ useParams: () => ({ id: "657515" }) }),
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="/" className={className}>
      {children}
    </a>
  ),
}));
vi.mock("@/features/player", () => ({ playSongs: vi.fn(), shuffleSongs: vi.fn() }));
vi.mock("@/features/concerts", () => ({ ArtistConcerts: () => null }));
vi.mock("@/shared/actions/ActionMenu", () => ({
  ActionMenu: () => null,
  ItemActionMenu: () => null,
}));
vi.mock("@/shared/components/RatingButtons", () => ({ RatingButtons: () => null }));
vi.mock("@/shared/components/MotionCover", () => ({ MotionCover: () => null }));
vi.mock("@/shared/tracks/TrackList", () => ({
  TrackList: ({ label }: { label: string }) => <div role="grid" aria-label={label} />,
}));
vi.mock("@/shared/components/CoverShelf", () => ({
  CoverShelf: ({ title }: { title: string }) => (
    <section aria-label={title}>
      <h2>{title}</h2>
    </section>
  ),
}));

const data = vi.hoisted(() => ({ page: undefined as CatalogArtist | undefined }));
vi.mock("./queries", () => ({
  useLibraryArtist: () => ({ data: undefined, isPending: false }),
  useCatalogArtist: () => ({ data: data.page, isPending: false, error: null }),
}));

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
  // The content column of a 1440px window.
  Object.defineProperty(HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get: () => 1144,
  });
  Element.prototype.scrollIntoView = vi.fn();
});
afterEach(cleanup);

const art = { small: "", medium: "", large: "" };

function album(id: string, name = id, releaseDate = "2016-05-03"): Album {
  return { id, name, releaseDate, art, artistName: "Radiohead" } as unknown as Album;
}

function shelf(view: string, albums: Album[], more = false): ArtistShelf {
  const kind = { playlists: ShelfKind.ShelfPlaylists, "similar-artists": ShelfKind.ShelfArtists }[
    view
  ];
  return {
    view,
    kind: kind ?? ShelfKind.ShelfAlbums,
    more,
    albums,
    playlists: [],
    videos: [],
    artists: [],
  };
}

function renderPage(shelves: ArtistShelf[], extra: Partial<CatalogArtist> = {}) {
  data.page = {
    artist: { id: "657515", name: "Radiohead", genre: "", art },
    header: { backdrop: "", colors: [], motion: "" },
    bio: "More than a band.\n\nFormed in 1985.",
    origin: "Abingdon, Oxfordshire, England",
    bornOrFormed: "1985",
    genres: ["Alternative"],
    url: "",
    latestRelease: album("latest", "Hail to the Thief (Live Recordings 2003-2009)"),
    topSongs: [],
    topSongsMore: false,
    shelves,
    ...extra,
  } as unknown as CatalogArtist;
  return render(<ArtistPage />, { wrapper: I18nRoot });
}

const band = (name: string) => screen.getByRole("region", { name });

describe("ArtistPage", () => {
  it("groups the shelves into Discography, Related and About, in that order", () => {
    renderPage([
      shelf("featured-albums", [album("ok", "OK Computer")]),
      shelf("full-albums", [album("ir", "In Rainbows")]),
      shelf("singles", [album("dd", "Daydreaming - Single")], true),
      shelf("appears-on-albums", [album("creep", "Creep (Very 2021 Rmx)")]),
      shelf("compilation-albums", [album("kam", "KID A MNESIA")]),
      shelf("playlists", []),
      shelf("similar-artists", []),
    ]);
    const [discography, related, about] = ["Discography", "Related", "About Radiohead"].map(band);
    const after = Node.DOCUMENT_POSITION_FOLLOWING;
    expect(discography.compareDocumentPosition(related) & after).toBe(after);
    expect(related.compareDocumentPosition(about) & after).toBe(after);

    expect(within(discography).getByRole("region", { name: "Albums" })).toBeTruthy();
    // Singles, live albums and compilations are the back catalogue's lists.
    const singles = within(discography).getByRole("grid", { name: "Singles and EPs" });
    expect(within(singles).getByText("Daydreaming")).toBeTruthy();
    expect(within(singles).getByText("2016 · Single")).toBeTruthy();
    expect(within(discography).getByRole("grid", { name: "Compilations" })).toBeTruthy();

    const appears = within(related).getByRole("grid", { name: "Appears on" });
    expect(within(appears).getByText("Radiohead · 2016")).toBeTruthy();
    expect(within(related).getByRole("region", { name: "Similar artists" })).toBeTruthy();

    // The essential albums lead beside the top songs, outside the bands.
    const essentials = screen.getByRole("grid", { name: "Essential albums" });
    expect(discography.contains(essentials)).toBe(false);
  });

  it("opens with the biography's first paragraph; Read more moves focus to About", () => {
    renderPage([shelf("full-albums", [album("ir")], true)]);
    const header = screen.getByRole("banner");
    expect(within(header).getByText("More than a band.")).toBeTruthy();
    expect(within(header).queryByText(/Formed in 1985/)).toBeNull();
    expect(within(header).getByText(/Hail to the Thief/)).toBeTruthy();

    fireEvent.click(within(header).getByRole("button", { name: "Read more" }));
    const about = band("About Radiohead");
    expect(document.activeElement).toBe(about);
    expect(within(about).getByText("Formed in 1985.")).toBeTruthy();
  });

  it("gives an artist with one release its count, and doesn't repeat the release in the header", () => {
    renderPage([shelf("singles", [album("one", "Am Sorry - Single")])], {
      latestRelease: album("one", "Am Sorry - Single"),
      genres: [],
      origin: "",
      bio: "",
      bornOrFormed: "",
    });
    const header = screen.getByRole("banner");
    expect(within(header).getByText("Artist · 1 release")).toBeTruthy();
    expect(within(header).queryByText("Am Sorry")).toBeNull();
    expect(within(band("Discography")).getByText("Am Sorry")).toBeTruthy();
    expect(screen.queryByRole("region", { name: /About/ })).toBeNull();
  });
});
