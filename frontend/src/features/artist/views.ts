/**
 * The artist views Sona shows (Go's View* names) and their titles, in
 * Sona's words. Apple's own titles are data and are not reused.
 */
export const artistViews = {
  "top-songs": "Top songs",
  "featured-albums": "Essential albums",
  "full-albums": "Albums",
  singles: "Singles and EPs",
  "live-albums": "Live albums",
  "compilation-albums": "Compilations",
  "appears-on-albums": "Appears on",
  playlists: "Playlists featuring",
  "music-videos": "Music videos",
  "top-music-videos": "Music videos",
  "similar-artists": "Similar artists",
} as const;

export type ArtistView = keyof typeof artistViews;

export function isArtistView(v: string): v is ArtistView {
  return Object.hasOwn(artistViews, v);
}

/** A shelf or see-all title: "Playlists featuring" takes the artist's name. */
export function viewTitle(view: ArtistView, artist: string): string {
  return view === "playlists" ? `Playlists featuring ${artist}` : artistViews[view];
}
