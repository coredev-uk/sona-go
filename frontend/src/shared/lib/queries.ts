const libraryPrefix = { album: "l.", artist: "r.", playlist: "p." };

/** Library ids carry their kind's prefix (Apple's convention); everything else is catalogue. */
export function isLibraryId(id: string, kind: keyof typeof libraryPrefix): boolean {
  return id.startsWith(libraryPrefix[kind]);
}
