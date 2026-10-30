import { QueryClient } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

/** Which of these cached queries an invalidation list marks stale. Test helper. */
export function staleAfter(
  invalidate: readonly (readonly unknown[])[],
  cached: readonly unknown[][],
) {
  const qc = new QueryClient();
  for (const key of cached) qc.setQueryData(key, 1);
  for (const key of invalidate) void qc.invalidateQueries({ queryKey: key, refetchType: "none" });
  return cached.filter((key) => qc.getQueryState(key)?.isInvalidated).map((k) => k.join("/"));
}

/** A cache with one query of each shape, for invalidation-plan tests. */
export const cachedKeys = [
  [...keys.auth],
  [...keys.settings],
  [...keys.diagnostics],
  [...keys.libraryCounts],
  [...keys.librarySync],
  [...keys.libraryRecent],
  [...keys.libraryPage("songs", "title", false, 0)],
  [...keys.libraryPage("albums", "dateAdded", true, 2)],
  [...keys.libraryPage("playlists", "title", false, 0)],
  [...keys.libraryAlbum("l.abc")],
  [...keys.libraryPlaylist("p.abc")],
  [...keys.catalogSearch("radiohead")],
  [...keys.catalogAlbum("1097861387")],
  [...keys.lyrics("1097862703")],
];
