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
