import { t } from "@lingui/core/macro";

/**
 * The artist views Sona shows (Go's View* names) and their titles, in
 * Sona's words. Apple's own titles are data and are not reused.
 */
export const artistViews = {
  "top-songs": () => t`Top songs`,
  "featured-albums": () => t`Essential albums`,
  "full-albums": () => t`Albums`,
  singles: () => t`Singles and EPs`,
  "live-albums": () => t`Live albums`,
  "compilation-albums": () => t`Compilations`,
  "appears-on-albums": () => t`Appears on`,
  playlists: () => t`Playlists featuring`,
  "music-videos": () => t`Music videos`,
  "top-music-videos": () => t`Music videos`,
  "similar-artists": () => t`Similar artists`,
} satisfies Record<string, () => string>;

export type ArtistView = keyof typeof artistViews;

export function isArtistView(v: string): v is ArtistView {
  return Object.hasOwn(artistViews, v);
}

/** A shelf or see-all title: "Playlists featuring" takes the artist's name. */
export function viewTitle(view: ArtistView, artist: string): string {
  return view === "playlists" ? t`Playlists featuring ${artist}` : artistViews[view]();
}
