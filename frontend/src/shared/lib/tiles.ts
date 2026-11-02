import type { Album, Artist, MusicVideo, Playlist } from "@bindings/services";

import type { CoverItem } from "@/shared/components/CoverGrid";
import { canPlayVideo, playVideo } from "@/shared/items/play";
import { yearOf } from "@/shared/lib/format";
import { releaseBadge } from "@/shared/lib/release";

/**
 * Cover tiles for the card-level DTOs, linking to each item's page. The
 * subtitle says what tells neighbouring tiles apart: an album's year on an
 * artist's own shelves (`by: "year"`), else its artist. An album not out
 * yet carries its release date on the cover.
 */
export function albumTile(a: Album, by: "artist" | "year" = "artist"): CoverItem {
  return {
    key: a.id,
    title: a.name,
    subtitle: by === "year" ? yearOf(a.releaseDate) : a.artistName,
    art: a.art.medium,
    motion: a.motion || undefined,
    link: { to: "/album/$id", params: { id: a.id } },
    target: { kind: "album", target: a },
    badge: releaseBadge(a),
  };
}

export function playlistTile(p: Playlist): CoverItem {
  return {
    key: p.id,
    title: p.name,
    subtitle: p.curatorName,
    art: p.art.medium,
    motion: p.motion || undefined,
    link: { to: "/playlist/$id", params: { id: p.id } },
    target: { kind: "playlist", target: p },
  };
}

export function artistTile(a: Artist): CoverItem {
  return {
    key: a.id,
    title: a.name,
    art: a.art.medium,
    motion: a.motion || undefined,
    shape: "round",
    link: { to: "/artist/$id", params: { id: a.id } },
    target: { kind: "artist", target: a },
  };
}

/** A music video's tile, with a play control where videos play (canPlayVideo). */
export function videoTile(v: MusicVideo, by: "artist" | "year" = "artist"): CoverItem {
  return {
    key: v.id,
    title: v.name,
    subtitle: by === "year" ? yearOf(v.releaseDate) : v.artistName,
    art: v.art.medium,
    aspect: "video",
    explicit: v.explicit,
    link: { to: "/music-video/$id", params: { id: v.catalogId || v.id } },
    play: canPlayVideo() ? { label: v.name, run: () => playVideo(v) } : undefined,
  };
}
