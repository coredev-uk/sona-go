import { PlaylistsService, Source, type TreePlaylist } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { FolderInput, ListMinus, ListPlus, Pencil, Pin, Trash2 } from "lucide-react";

import {
  describeItems,
  type ActionDef,
  type ActionTargetKind,
  type ActionTargets,
  type ItemActionDef,
  type MenuData,
} from "@/shared/actions/registry";

import { addToPlaylist, deletePlaylist, pin, playlistItems, unpin } from "./commands";
import { showDialog } from "./dialogs";
import { boundQueryClient, removeTracks } from "./track-edits";
import { playlistSection } from "./tree";

/** The library ID an item can be pinned by ("" when it is not in the library). */
export function libraryIdOf(t: { id: string; source: Source; libraryId: string }): string {
  return t.libraryId || (t.source === Source.SourceLibrary ? t.id : "");
}

/** The pinned library IDs, read when a menu opens. */
function pinnedData<K extends ActionTargetKind>(): MenuData<K, ReadonlySet<string>> {
  return {
    id: "playlists.pinned",
    load: () =>
      PlaylistsService.Pins().then(
        (pins) => new Set(pins.map((p) => p.id)),
        () => new Set<string>(),
      ),
  };
}

/** The library playlists by ID, with their folder and permissions. */
const treeData: MenuData<"playlist", ReadonlyMap<string, TreePlaylist>> = {
  id: "playlists.tree",
  load: () =>
    PlaylistsService.Tree().then(
      (t) => new Map(t.playlists.map((p) => [p.playlist.id, p])),
      () => new Map<string, TreePlaylist>(),
    ),
};

function addTo<K extends "song" | "album" | "playlist">(target: K): ActionDef<K> {
  return {
    id: `playlists.add-to.${target}`,
    target,
    label: () => t`Add to playlist…`,
    icon: ListPlus,
    group: "item",
    order: 40,
    run: (item) =>
      addToPlaylist([{ kind: target, id: item.id, source: item.source }], `“${item.name}”`),
  };
}

/** Library items Apple can pin. */
type Pinnable = "song" | "album" | "artist" | "playlist";

function pinAction<K extends Pinnable>(target: K): ActionDef<K> {
  const pinned = pinnedData<K>();
  const isPinned = (item: ActionTargets[K], data: ReadonlySet<string> | undefined) =>
    !!data?.has(libraryIdOf(item));
  return {
    id: `playlists.pin.${target}`,
    target,
    label: (item, ctx) => (isPinned(item, ctx.data(pinned)) ? t`Unpin` : t`Pin to sidebar`),
    icon: Pin,
    group: "item",
    order: 60,
    isAvailable: (item) => libraryIdOf(item) !== "",
    data: [pinned],
    run: (item, ctx) => {
      const id = libraryIdOf(item);
      if (isPinned(item, ctx.data(pinned))) void unpin(id, item.name);
      else void pin(id, item.name);
    },
  };
}

const isLibraryPlaylist = (p: ActionTargets["playlist"]) => p.source === Source.SourceLibrary;

/**
 * Item actions: add songs, albums and playlists to a playlist; pin library
 * items to the sidebar; edit, move and delete library playlists. A
 * selection can be added to a playlist, or removed from the playlist it is
 * listed in, at once.
 */
export const playlistActions: readonly ItemActionDef[] = [
  addTo("song"),
  {
    id: "playlists.remove-from",
    target: "song",
    label: () => t`Remove from playlist`,
    icon: ListMinus,
    group: "item",
    order: 45,
    isAvailable: (_, ctx) => ctx.container?.kind === "playlist" && ctx.container.editable,
    run: (song, ctx) => {
      const qc = boundQueryClient();
      if (qc && ctx.container) removeTracks(qc, ctx.container.id, [ctx.container.index], [song]);
    },
  },
  addTo("album"),
  addTo("playlist"),
  {
    id: "playlists.add-to.many",
    target: "selection",
    kinds: ["song", "album", "playlist"],
    label: () => t`Add to playlist…`,
    icon: ListPlus,
    group: "item",
    order: 40,
    run: (items) => addToPlaylist(playlistItems(items), describeItems(items)),
  },
  {
    id: "playlists.remove-from.many",
    target: "selection",
    kinds: ["song"],
    label: () => t`Remove from playlist`,
    icon: ListMinus,
    group: "item",
    order: 45,
    isAvailable: (_, ctx) => ctx.container?.kind === "playlist" && ctx.container.editable,
    run: (items, ctx) => {
      const qc = boundQueryClient();
      const songs = items.flatMap((i) => (i.kind === "song" ? [i.target] : []));
      if (qc && ctx.container) removeTracks(qc, ctx.container.id, ctx.container.indices, songs);
    },
  },
  pinAction("song"),
  pinAction("album"),
  pinAction("artist"),
  pinAction("playlist"),
  {
    id: "playlists.edit",
    target: "playlist",
    label: () => t`Edit details…`,
    icon: Pencil,
    group: "item",
    order: 70,
    isAvailable: (p) => isLibraryPlaylist(p) && p.canEdit,
    run: (p) =>
      void PlaylistsService.Playlist(p.id).then(
        (d) =>
          showDialog({
            kind: "details",
            id: p.id,
            name: d.playlist.name,
            description: d.playlist.description,
            isPublic: d.isPublic,
          }),
        () =>
          showDialog({
            kind: "details",
            id: p.id,
            name: p.name,
            description: p.description,
            isPublic: false,
          }),
      ),
  },
  {
    id: "playlists.move",
    target: "playlist",
    label: () => t`Move to folder…`,
    icon: FolderInput,
    group: "item",
    order: 75,
    // Folders hold the user's own playlists; the others sit in their sidebar section.
    isAvailable: (p) => isLibraryPlaylist(p) && p.canEdit,
    data: [treeData],
    run: (p, ctx) =>
      showDialog({
        kind: "move",
        id: p.id,
        name: p.name,
        folderId: ctx.data(treeData)?.get(p.id)?.folderId ?? "",
      }),
  },
  {
    id: "playlists.delete",
    target: "playlist",
    label: () => t`Delete from library…`,
    icon: Trash2,
    group: "item",
    order: 90,
    // Apple keeps the personal mixes it makes; only playlists the user added can go.
    isAvailable: (p) => isLibraryPlaylist(p) && playlistSection(p) !== "madeForYou",
    isEnabled: (p, ctx) => ctx.data(treeData)?.get(p.id)?.canDelete ?? false,
    data: [treeData],
    run: (p, ctx) =>
      void deletePlaylist(p.id, p.name).then((ok) => {
        if (ok && location.hash.includes(`/playlist/${p.id}`))
          void ctx.navigate({ to: "/library/playlists" });
      }),
  },
];
