/** app:open payload: a parsed music:// / itms:// / music.apple.com link. */
export interface OpenTarget {
  kind: string;
  id: string;
  albumId?: string;
}

/** The route an app:open target lands on. */
export type OpenRoute =
  | { to: "/album/$id"; params: { id: string }; search: { song?: string } }
  | { to: "/song/$id"; params: { id: string } }
  | { to: "/artist/$id"; params: { id: string } }
  | { to: "/playlist/$id"; params: { id: string } }
  | { to: "/music-video/$id"; params: { id: string } }
  | { to: "/station/$id"; params: { id: string } }
  | { to: "/curator/$id"; params: { id: string } };

export function routeForOpen(t: OpenTarget): OpenRoute | undefined {
  if (!t.id) return undefined;
  switch (t.kind) {
    case "album":
    case "library-album":
      return { to: "/album/$id", params: { id: t.id }, search: {} };
    case "song":
      // A song shared within its album opens there with the track marked;
      // a bare song link opens the song page.
      return t.albumId
        ? { to: "/album/$id", params: { id: t.albumId }, search: { song: t.id } }
        : { to: "/song/$id", params: { id: t.id } };
    case "artist":
      return { to: "/artist/$id", params: { id: t.id } };
    case "playlist":
    case "library-playlist":
      return { to: "/playlist/$id", params: { id: t.id } };
    case "music-video":
      return { to: "/music-video/$id", params: { id: t.id } };
    case "station":
      return { to: "/station/$id", params: { id: t.id } };
    case "curator":
      return { to: "/curator/$id", params: { id: t.id } };
    default:
      return undefined;
  }
}
