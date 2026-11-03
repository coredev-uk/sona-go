import type { Playlist, PlaylistTree, TreePlaylist } from "@bindings/services";
import { describe, expect, it } from "vitest";

import {
  addTargets,
  buildTree,
  folderPaths,
  playlistSection,
  sidebarPlaylists,
  type TreeNode,
} from "./tree";

function pl(
  id: string,
  name: string,
  folderId: string,
  position: number,
  canEdit = true,
  catalogId = "",
): TreePlaylist {
  return {
    playlist: { id, name, canEdit, catalogId } as Playlist,
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

describe("buildTree with pins", () => {
  // A folder shows how many of its playlists were left out: "Mixes+1[…]".
  const withPins = (nodes: TreeNode[]): string[] =>
    nodes.map((n) =>
      n.kind === "folder"
        ? `${n.folder.name}${n.hidden ? `+${n.hidden}` : ""}[${withPins(n.children).join(",")}]`
        : n.item.playlist.name,
    );

  it("leaves pinned playlists out at every level and keeps their folders", () => {
    expect(withPins(buildTree(tree, new Set(["p.a", "p.in", "p.deep"])))).toEqual([
      "Mixes+1[Old+1[]]",
      "Orphan[]",
      "Beta",
      "Made for you",
    ]);
  });

  it("counts every pinned playlist of a folder", () => {
    const running: PlaylistTree = {
      folders: [{ id: "f.run", parentId: "", name: "Running", position: 0 }],
      playlists: [pl("p.t", "Tempo", "f.run", 0), pl("p.l", "Long", "f.run", 1)],
    };
    expect(withPins(buildTree(running, new Set(["p.t", "p.l"])))).toEqual(["Running+2[]"]);
    expect(withPins(buildTree(running, new Set(["p.l"])))).toEqual(["Running+1[Tempo]"]);
  });

  it("puts an unpinned playlist back in its place", () => {
    const pinned = new Set(["p.b", "p.in"]);
    expect(withPins(buildTree(tree, pinned))).toEqual([
      "Mixes+1[Old[Deep]]",
      "Orphan[]",
      "Alpha",
      "Made for you",
    ]);
    pinned.delete("p.b");
    pinned.delete("p.in");
    expect(withPins(buildTree(tree, pinned))).toEqual(withPins(buildTree(tree)));
    expect(withPins(buildTree(tree))).toEqual([
      "Mixes[Old[Deep],Inside]",
      "Orphan[]",
      "Alpha",
      "Beta",
      "Made for you",
    ]);
  });

  it("ignores pins that are not playlists in the tree", () => {
    expect(withPins(buildTree(tree, new Set(["l.album", "r.artist", "p.gone"])))).toEqual(
      withPins(buildTree(tree)),
    );
  });

  it("leaves out a pinned playlist whose folder is unknown", () => {
    const lost: PlaylistTree = {
      folders: [],
      playlists: [pl("p.x", "Stray", "f.gone", 0), pl("p.y", "Kept", "", 1)],
    };
    expect(withPins(buildTree(lost, new Set(["p.x"])))).toEqual(["Kept"]);
  });
});

describe("playlistSection", () => {
  it("names Apple's personal mixes by their catalog ID, editable or not", () => {
    expect(playlistSection({ catalogId: "pl.pm-4c1f", canEdit: false })).toBe("madeForYou");
    expect(playlistSection({ catalogId: "pl.pm-4c1f", canEdit: true })).toBe("madeForYou");
  });

  it("puts playlists the user cannot edit under Apple Music, whoever made them", () => {
    expect(playlistSection({ catalogId: "pl.f4d106fe", canEdit: false })).toBe("apple"); // editorial
    expect(playlistSection({ catalogId: "pl.u-8aZ", canEdit: false })).toBe("apple"); // someone's shared playlist
    expect(playlistSection({ catalogId: "pl.rp-1", canEdit: false })).toBe("apple"); // Replay is not a mix
    expect(playlistSection({ catalogId: "", canEdit: false })).toBe("apple");
  });

  it("keeps editable playlists with the user's own, shared or private", () => {
    expect(playlistSection({ catalogId: "", canEdit: true })).toBe("own");
    expect(playlistSection({ catalogId: "pl.u-8aZ", canEdit: true })).toBe("own"); // collaborative
  });
});

describe("sidebarPlaylists", () => {
  const library: PlaylistTree = {
    folders: [{ id: "f.run", parentId: "", name: "Running", position: 0 }],
    playlists: [
      pl("p.own", "Mine", "", 5),
      pl("p.fav", "Favourites Mix", "", 2, false, "pl.pm-1"),
      pl("p.new", "New Music Mix", "", 1, false, "pl.pm-2"),
      pl("p.zed", "Zen", "", 7, false, "pl.f4d"),
      pl("p.abe", "Alt Rock", "f.run", 0, false, "pl.f4e"),
      pl("p.tempo", "Tempo", "f.run", 1),
    ],
  };
  const names = (items: TreePlaylist[]) => items.map((i) => i.playlist.name);
  const withHidden = (nodes: TreeNode[]): string[] =>
    nodes.map((n) =>
      n.kind === "folder"
        ? `${n.folder.name}${n.hidden ? `+${n.hidden}` : ""}[${withHidden(n.children).join(",")}]`
        : n.item.playlist.name,
    );

  it("splits the library into the three sections", () => {
    const s = sidebarPlaylists(library);
    expect(names(s.madeForYou)).toEqual(["New Music Mix", "Favourites Mix"]); // Apple's order
    expect(names(s.apple)).toEqual(["Alt Rock", "Zen"]); // by name
    expect(withHidden(s.own)).toEqual(["Running+1[Tempo]", "Mine"]);
  });

  it("lists a pinned playlist under no section, whichever it belongs to", () => {
    const s = sidebarPlaylists(library, new Set(["p.fav", "p.zed", "p.own", "p.tempo"]));
    expect(names(s.madeForYou)).toEqual(["New Music Mix"]);
    expect(names(s.apple)).toEqual(["Alt Rock"]);
    expect(withHidden(s.own)).toEqual(["Running+2[]"]);
  });

  it("lists it again in its own section once unpinned", () => {
    const pinned = new Set(["p.fav"]);
    expect(names(sidebarPlaylists(library, pinned).madeForYou)).toEqual(["New Music Mix"]);
    pinned.clear();
    expect(names(sidebarPlaylists(library, pinned).madeForYou)).toEqual([
      "New Music Mix",
      "Favourites Mix",
    ]);
  });

  it("leaves a section empty when the library has none for it", () => {
    const s = sidebarPlaylists({ folders: [], playlists: [pl("p.own", "Mine", "", 0)] });
    expect(s.madeForYou).toEqual([]);
    expect(s.apple).toEqual([]);
    expect(withHidden(s.own)).toEqual(["Mine"]);
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
