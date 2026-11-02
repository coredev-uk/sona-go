import type { PlaylistCollaboration, SocialProfile, Song } from "@bindings/services";

/**
 * Who added each of `tracks`, by row: Apple names the adder of a
 * collaborative playlist's songs by catalogue ID in playlist order, so each
 * row takes the next unclaimed attribution of its song (a song added twice
 * keeps both adders). Rows Apple says nothing about are `undefined`.
 */
export function addedByRows(
  tracks: readonly Song[],
  c: PlaylistCollaboration,
): (SocialProfile | undefined)[] {
  const people = new Map<string, SocialProfile>();
  for (const p of [...c.collaborators, ...c.former]) people.set(p.id, p);
  const queue = new Map<string, string[]>();
  for (const a of c.addedBy) {
    const q = queue.get(a.songId);
    if (q) q.push(a.profileId);
    else queue.set(a.songId, [a.profileId]);
  }
  return tracks.map((t) => {
    const id = queue.get(t.catalogId || t.id)?.shift();
    return id ? people.get(id) : undefined;
  });
}
