import type { Album, Artist, MusicVideo, Playlist } from "@bindings/services";

import type { CoverItem } from "@/shared/components/CoverGrid";
import { yearOf } from "@/shared/lib/format";

/**
 * Cover tiles for the card-level DTOs, linking to each item's page. The
 * subtitle says what tells neighbouring tiles apart: an album's year on an
 * artist's own shelves (`by: "year"`), else its artist.
 */
export function albumTile(a: Album, by: "artist" | "year" = "artist"): CoverItem {
  return {
    key: a.id,
    title: a.name,
    subtitle: by === "year" ? yearOf(a.releaseDate) : a.artistName,
    art: a.art.medium,
    link: { to: "/album/$id", params: { id: a.id } },
  };
}

export function playlistTile(p: Playlist): CoverItem {
  return {
    key: p.id,
    title: p.name,
    subtitle: p.curatorName,
    art: p.art.medium,
    link: { to: "/playlist/$id", params: { id: p.id } },
  };
}

export function artistTile(a: Artist): CoverItem {
  return {
    key: a.id,
    title: a.name,
    art: a.art.medium,
    shape: "round",
    link: { to: "/artist/$id", params: { id: a.id } },
  };
}

export function videoTile(v: MusicVideo, by: "artist" | "year" = "artist"): CoverItem {
  return {
    key: v.id,
    title: v.name,
    subtitle: by === "year" ? yearOf(v.releaseDate) : v.artistName,
    art: v.art.medium,
    aspect: "video",
    link: { to: "/music-video/$id", params: { id: v.catalogId || v.id } },
  };
}
