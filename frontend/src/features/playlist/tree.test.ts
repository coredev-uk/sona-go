import type { Playlist, PlaylistTree, TreePlaylist } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { addTargets, buildTree, folderPaths, type TreeNode } from "./tree";

function pl(
  id: string,
  name: string,
  folderId: string,
  position: number,
  canEdit = true,
): TreePlaylist {
  return {
    playlist: { id, name, canEdit } as Playlist,
    folderId,
    position,
    canDelete: true,
    favorited: false,
  };
}

const tree: PlaylistTree = {
  folders: [
    { id: "f.old", parentId: "f.mix", name: "Old", position: 0 },
    { id: "f.mix", parentId: "", name: "Mixes", position: 1 },
    { id: "f.lost", parentId: "f.gone", name: "Orphan", position: 9 },
  ],
  playlists: [
    pl("p.b", "Beta", "", 2),
    pl("p.a", "Alpha", "", 0),
    pl("p.m", "Made for you", "", 3, false),
    pl("p.in", "Inside", "f.mix", 1),
    pl("p.deep", "Deep", "f.old", 0),
  ],
};

const outline = (nodes: TreeNode[]): string[] =>
  nodes.map((n) =>
    n.kind === "folder"
      ? `${n.folder.name}[${outline(n.children).join(",")}]`
      : n.item.playlist.name,
  );

describe("buildTree", () => {
  it("lists folders before playlists at every level, each by Apple's position", () => {
    expect(outline(buildTree(tree))).toEqual([
      "Mixes[Old[Deep],Inside]",
      "Orphan[]",
      "Alpha",
      "Beta",
      "Made for you",
    ]);
  });

  it("puts folders first in nested folders too, and keeps empty folders", () => {
    const nested: PlaylistTree = {
      folders: [
        { id: "f.top", parentId: "", name: "Top", position: 5 },
        { id: "f.b", parentId: "f.top", name: "B", position: 3 },
        { id: "f.a", parentId: "f.top", name: "A", position: 3 },
        { id: "f.empty", parentId: "f.b", name: "Empty", position: 2 },
      ],
      playlists: [
        pl("p.1", "One", "f.top", 0),
        pl("p.2", "Two", "f.b", 1),
        pl("p.0", "Zero", "", 0),
      ],
    };
    expect(outline(buildTree(nested))).toEqual(["Top[A[],B[Empty[],Two],One]", "Zero"]);
  });

  it("orders a level with only folders or only playlists by position, then name", () => {
    const onlyFolders: PlaylistTree = {
      folders: [
        { id: "f.2", parentId: "", name: "Second", position: 2 },
        { id: "f.1", parentId: "", name: "First", position: 1 },
      ],
      playlists: [pl("p.y", "Yankee", "f.1", 1), pl("p.x", "Xray", "f.1", 1)],
    };
    expect(outline(buildTree(onlyFolders))).toEqual(["First[Xray,Yankee]", "Second[]"]);
    expect(outline(buildTree({ folders: [], playlists: [] }))).toEqual([]);
  });

  it("lists folder paths depth-first", () => {
    expect(folderPaths(buildTree(tree)).map((f) => f.path)).toEqual([
      "Mixes",
      "Mixes / Old",
      "Orphan",
    ]);
  });
});

describe("addTargets", () => {
  const nodes = buildTree(tree);

  it("groups editable playlists by folder, recent ones first", () => {
    const t = addTargets(nodes, ["p.deep", "p.m", "p.unknown"], "");
    expect(t.recent.map((p) => p.playlist.id)).toEqual(["p.deep"]);
    expect(t.groups.map((g) => [g.path, g.items.map((p) => p.playlist.name)])).toEqual([
      ["", ["Alpha", "Beta"]],
      ["Mixes", ["Inside"]],
      ["Mixes / Old", ["Deep"]],
    ]);
  });

  it("filters by any part of the name and hides recents while filtering", () => {
    const t = addTargets(nodes, ["p.deep"], "  E ");
    expect(t.recent).toEqual([]);
    expect(t.groups.flatMap((g) => g.items.map((p) => p.playlist.name))).toEqual([
      "Beta",
      "Inside",
      "Deep",
    ]);
  });
});
