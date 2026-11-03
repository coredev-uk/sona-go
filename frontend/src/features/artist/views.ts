import { t } from "@lingui/core/macro";

/**
 * The artist views Sona shows (Go's View* names) and their titles, in
 * Apple's wording (sentence case), each naming the artist.
 */
export const artistViews = {
  "top-songs": (artist) => t`Top songs by ${artist}`,
  "featured-albums": (artist) => t`Essential albums by ${artist}`,
  "full-albums": (artist) => t`Albums by ${artist}`,
  singles: (artist) => t`Singles and EPs featuring ${artist}`,
  "live-albums": (artist) => t`Live albums by ${artist}`,
  "compilation-albums": (artist) => t`Compilations featuring ${artist}`,
  "appears-on-albums": (artist) => t`Albums featuring ${artist}`,
  playlists: (artist) => t`Playlists featuring ${artist}`,
  "music-videos": (artist) => t`Music videos by ${artist}`,
  "top-music-videos": (artist) => t`Music videos by ${artist}`,
  "similar-artists": (artist) => t`Similar artists to ${artist}`,
} satisfies Record<string, (artist: string) => string>;

export type ArtistView = keyof typeof artistViews;

export function isArtistView(v: string): v is ArtistView {
  return Object.hasOwn(artistViews, v);
}

/** A shelf or see-all title, naming the artist. */
export function viewTitle(view: ArtistView, artist: string): string {
  return artistViews[view](artist);
}
