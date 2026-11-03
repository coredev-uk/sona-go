import type { Playlist, PlaylistFolder, PlaylistTree, TreePlaylist } from "@bindings/services";

export type TreeNode =
  | {
      kind: "folder";
      folder: PlaylistFolder;
      children: TreeNode[];
      /** How many of its playlists are left out because the sidebar lists them elsewhere. */
      hidden: number;
    }
  | { kind: "playlist"; item: TreePlaylist };

const position = (n: TreeNode) => (n.kind === "folder" ? n.folder.position : n.item.position);
const name = (n: TreeNode) => (n.kind === "folder" ? n.folder.name : n.item.playlist.name);

const NONE: ReadonlySet<string> = new Set();

/** The sidebar section a library playlist is listed in (unless it is pinned). */
export type PlaylistSection = "madeForYou" | "apple" | "own";

/** Catalog ID prefix of Apple's personal mixes (Favourites Mix, New Music Mix, Chill Mix, …). */
const PERSONAL_MIX = "pl.pm-";

/**
 * Where a library playlist belongs. Apple's personal mixes are catalog
 * playlists `pl.pm-…` (playlistType `personal-mix`), so the library copy
 * names one by its catalog ID. Any other playlist the user cannot edit was
 * added from Apple Music (or another curator); the rest are the user's own.
 */
export function playlistSection(p: Pick<Playlist, "catalogId" | "canEdit">): PlaylistSection {
  if (p.catalogId.startsWith(PERSONAL_MIX)) return "madeForYou";
  return p.canEdit ? "own" : "apple";
}

/**
 * The folder tree: at every level folders come first, then playlists, each
 * group in Apple's position order (then by name). Folders whose parent is
 * unknown, and playlists in an unknown folder, sit at the top level.
 * Playlists in `hidden` (library IDs) are left out, as the sidebar lists
 * them in another section (Pinned, Made for You, Apple Music); their
 * folders stay, counting them in `hidden`.
 */
export function buildTree(tree: PlaylistTree, hidden = NONE): TreeNode[] {
  const folders = new Map<string, TreeNode & { kind: "folder" }>();
  for (const f of tree.folders)
    folders.set(f.id, { kind: "folder", folder: f, children: [], hidden: 0 });
  const root: TreeNode[] = [];
  const parentList = (id: string) => (id && folders.get(id)?.children) || root;
  for (const node of folders.values()) {
    // A folder cannot hold itself (guards bad data from a cycle).
    if (node.folder.parentId === node.folder.id) root.push(node);
    else parentList(node.folder.parentId).push(node);
  }
  for (const item of tree.playlists) {
    if (!hidden.has(item.playlist.id)) parentList(item.folderId).push({ kind: "playlist", item });
    else {
      const folder = folders.get(item.folderId);
      if (folder) folder.hidden++;
    }
  }
  const sort = (list: TreeNode[]) => {
    list.sort(
      (a, b) =>
        (a.kind === b.kind ? 0 : a.kind === "folder" ? -1 : 1) ||
        position(a) - position(b) ||
        name(a).localeCompare(name(b)),
    );
    for (const n of list) if (n.kind === "folder") sort(n.children);
  };
  sort(root);
  return root;
}

export interface SidebarPlaylists {
  /** Apple's personal mixes, in Apple's order. */
  madeForYou: TreePlaylist[];
  /** Playlists the user added but cannot edit, by name. */
  apple: TreePlaylist[];
  /** The user's own playlists and folders. */
  own: TreeNode[];
}

const byPosition = (a: TreePlaylist, b: TreePlaylist) =>
  a.position - b.position || a.playlist.name.localeCompare(b.playlist.name);
const byName = (a: TreePlaylist, b: TreePlaylist) => a.playlist.name.localeCompare(b.playlist.name);

/**
 * The sidebar's playlist sections. A pinned playlist (library IDs in
 * `pinned`) is in none of them: it shows once, under Pinned, whatever its
 * section (#47). Folders keep to the user's own playlists; a folder's
 * non-editable playlists are listed in their section and counted as left
 * out of the folder.
 */
export function sidebarPlaylists(tree: PlaylistTree, pinned = NONE): SidebarPlaylists {
  const madeForYou: TreePlaylist[] = [];
  const apple: TreePlaylist[] = [];
  const hidden = new Set(pinned);
  for (const item of tree.playlists) {
    const section = playlistSection(item.playlist);
    if (section === "own") continue;
    hidden.add(item.playlist.id);
    if (!pinned.has(item.playlist.id)) (section === "madeForYou" ? madeForYou : apple).push(item);
  }
  return {
    madeForYou: madeForYou.sort(byPosition),
    apple: apple.sort(byName),
    own: buildTree(tree, hidden),
  };
}

/** Every folder with its path ("Mixes / Old"), depth-first in tree order. */
export function folderPaths(
  nodes: TreeNode[],
  prefix = "",
): { folder: PlaylistFolder; path: string }[] {
  return nodes.flatMap((n) => {
    if (n.kind !== "folder") return [];
    const path = prefix ? `${prefix} / ${n.folder.name}` : n.folder.name;
    return [{ folder: n.folder, path }, ...folderPaths(n.children, path)];
  });
}

export interface TargetGroup {
  /** "" for the top level, else the folder path. */
  path: string;
  items: TreePlaylist[];
}

/**
 * The playlists songs can be added to (editable ones), grouped by folder in
 * tree order and filtered by `query` (case-insensitive, any part of the
 * name), plus Apple's recently used ones first when not filtering.
 */
export function addTargets(
  nodes: TreeNode[],
  recentIds: readonly string[],
  query: string,
): { recent: TreePlaylist[]; groups: TargetGroup[] } {
  const q = query.trim().toLocaleLowerCase();
  const match = (p: TreePlaylist) =>
    p.playlist.canEdit && (!q || p.playlist.name.toLocaleLowerCase().includes(q));
  const groups: TargetGroup[] = [];
  const walk = (list: TreeNode[], path: string) => {
    const items = list.flatMap((n) => (n.kind === "playlist" && match(n.item) ? [n.item] : []));
    if (items.length) groups.push({ path, items });
    for (const n of list) {
      if (n.kind === "folder")
        walk(n.children, path ? `${path} / ${n.folder.name}` : n.folder.name);
    }
  };
  walk(nodes, "");
  const byId = new Map(groups.flatMap((g) => g.items.map((i) => [i.playlist.id, i] as const)));
  const recent = q
    ? []
    : recentIds.flatMap((id) => {
        const p = byId.get(id);
        return p ? [p] : [];
      });
  return { recent: recent.slice(0, 5), groups };
}
