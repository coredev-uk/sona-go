import { z } from "zod";

/** A library list's order: ?sort=…&desc=true. Unknown orders use the list's default. */
export function listSearch<const S extends readonly [string, ...string[]]>(sorts: S) {
  return z.object({
    sort: z.enum(sorts).optional().catch(undefined),
    desc: z.boolean().optional().catch(undefined),
  });
}

export const songSorts = ["title", "artist", "album", "dateAdded"] as const;
export const albumSorts = ["dateAdded", "title", "artist", "releaseDate"] as const;
export const playlistSorts = ["dateAdded", "title", "lastModified"] as const;

/** Which library albums to show: standard albums, or singles and EPs. No type shows all. */
export const albumTypes = ["albums", "singles"] as const;
export type AlbumType = (typeof albumTypes)[number];

/** The albums list's order and type: ?sort=…&desc=true&type=singles. Unknown values are dropped. */
export const albumsSearch = listSearch(albumSorts).extend({
  type: z.enum(albumTypes).optional().catch(undefined),
});
