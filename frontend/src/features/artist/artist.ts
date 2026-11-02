import { plural, t } from "@lingui/core/macro";
import { ShelfKind, type ArtistShelf, type CatalogArtist } from "@bindings/services";

import type { ArtistView } from "./views";

/** Top songs on the page; the rest are behind "See all". */
export const TOP_ROWS = 6;
/** Essential albums beside top songs. */
export const ESSENTIAL_ROWS = 6;
/** Releases in each catalogue list before "See all". */
export const CATALOGUE_ROWS = 5;

/** Content width from which the header and the lead have a second column. */
const WIDE = 900;
/** Content width under which the portrait drops to its small size. */
const NARROW = 720;
/** A catalogue column's smallest width and the gap between columns. */
const COLUMN_MIN = 260;
export const COLUMN_GAP = 32;

/** How the page lays out at a content width (0 before the first layout). */
export function layoutFor(width: number) {
  const wide = width >= WIDE;
  return {
    /** The latest release beside the header's text, essential albums beside top songs. */
    wide,
    portrait: width > 0 && width < NARROW ? 136 : 200,
    /** The latest release's cover: in the header's second column, or a row under the actions. */
    slip: wide ? 112 : 56,
  };
}

/** How many catalogue lists sit side by side: as many as fit, at most one each. */
export function catalogueColumns(width: number, lists: number): number {
  const fit = Math.floor((width + COLUMN_GAP) / (COLUMN_MIN + COLUMN_GAP));
  return Math.max(1, Math.min(lists, fit));
}

/** The page's bands and the artist views each holds, in the order they show. */
export const bandViews = {
  /** Cover tiles: the artist's albums. */
  albums: ["full-albums"],
  /** Compact lists side by side, the label's back catalogue. */
  catalogue: ["singles", "live-albums", "compilation-albums"],
  videos: ["top-music-videos", "music-videos"],
  /** A compact list, like the catalogue: albums by others the artist is on. */
  appears: ["appears-on-albums"],
  related: ["playlists", "similar-artists"],
} satisfies Record<string, ArtistView[]>;

/** The artist's shelves for `views`, in that order (Apple sends each at most once). */
export function shelvesFor(page: CatalogArtist, views: readonly ArtistView[]): ArtistShelf[] {
  return views.flatMap((v) => page.shelves.filter((sh) => sh.view === v));
}

/** Videos come as Apple's "top" pick or the full list; the page shows one shelf. */
export function videoShelf(page: CatalogArtist): ArtistShelf | undefined {
  return shelvesFor(page, bandViews.videos)[0];
}

/** The essential albums Apple picks for the artist, for the lead. */
export function essentials(page: CatalogArtist): ArtistShelf | undefined {
  return page.shelves.find(
    (sh) => sh.view === "featured-albums" && sh.kind === ShelfKind.ShelfAlbums,
  );
}

/** The artist's own release views: complete when none of them has more to fetch. */
const ownReleases: readonly string[] = [...bandViews.albums, ...bandViews.catalogue];

/**
 * How many releases the artist has, when the page holds all of them (no
 * own-release shelf has more to fetch): a small artist's whole output.
 * `undefined` when Apple has more than the page shows.
 */
export function releaseCount(page: CatalogArtist): number | undefined {
  const own = page.shelves.filter((sh) => ownReleases.includes(sh.view));
  if (own.some((sh) => sh.more)) return undefined;
  const ids = new Set(own.flatMap((sh) => sh.albums.map((a) => a.id)));
  if (page.latestRelease) ids.add(page.latestRelease.id);
  return ids.size;
}

/**
 * The artist's catalogue line (docs/design.md): `Artist · Alternative ·
 * Abingdon, Oxfordshire, England`, with the release count when the page
 * holds every release (`Artist · 1 release`). `extra` is a library line.
 */
export function artistLine(genre: string, origin: string, releases?: number, extra?: string) {
  const count = releases ? plural(releases, { one: "# release", other: "# releases" }) : "";
  return [t`Artist`, genre, origin, count, extra].filter(Boolean).join(" · ");
}

/** The biography's first paragraph, for the header. */
export function bioLead(bio: string): string {
  return bio.split(/\n{2,}/)[0]?.trim() ?? "";
}

/**
 * A single's or EP's title without Apple's " - Single" / " - EP" suffix,
 * and which it was: the catalogue lists say the kind in their own line.
 */
export function releaseTitle(name: string): { title: string; kind?: "single" | "ep" } {
  const m = name.match(/^(.+?) - (Single|EP)$/);
  if (!m) return { title: name };
  return { title: m[1], kind: m[2] === "EP" ? "ep" : "single" };
}
