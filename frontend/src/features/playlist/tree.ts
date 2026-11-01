import type { PlaylistFolder, PlaylistTree, TreePlaylist } from "@bindings/services";

export type TreeNode =
  | {
      kind: "folder";
      folder: PlaylistFolder;
      children: TreeNode[];
      /** How many of its playlists are left out because they are pinned. */
      pinned: number;
    }
  | { kind: "playlist"; item: TreePlaylist };

const position = (n: TreeNode) => (n.kind === "folder" ? n.folder.position : n.item.position);
const name = (n: TreeNode) => (n.kind === "folder" ? n.folder.name : n.item.playlist.name);

const NONE: ReadonlySet<string> = new Set();

/**
 * The folder tree: at every level folders come first, then playlists, each
 * group in Apple's position order (then by name). Folders whose parent is
 * unknown, and playlists in an unknown folder, sit at the top level.
 * Playlists in `pinned` (library IDs) are left out, as the sidebar shows
 * them under Pinned; their folders stay, counting them in `pinned`.
 */
export function buildTree(tree: PlaylistTree, pinned = NONE): TreeNode[] {
  const folders = new Map<string, TreeNode & { kind: "folder" }>();
  for (const f of tree.folders)
    folders.set(f.id, { kind: "folder", folder: f, children: [], pinned: 0 });
  const root: TreeNode[] = [];
  const parentList = (id: string) => (id && folders.get(id)?.children) || root;
  for (const node of folders.values()) {
    // A folder cannot hold itself (guards bad data from a cycle).
    if (node.folder.parentId === node.folder.id) root.push(node);
    else parentList(node.folder.parentId).push(node);
  }
  for (const item of tree.playlists) {
    if (!pinned.has(item.playlist.id)) parentList(item.folderId).push({ kind: "playlist", item });
    else {
      const folder = folders.get(item.folderId);
      if (folder) folder.pinned++;
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
