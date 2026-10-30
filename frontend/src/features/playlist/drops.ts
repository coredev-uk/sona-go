import type { Pin } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";

import { describeItems } from "@/shared/actions/registry";
import { useDropTarget, type DragPayload } from "@/shared/dnd/drag";

import { libraryIdOf } from "./actions";
import { addToPlaylistNow, pin, playlistItems } from "./commands";
import { showDialog } from "./dialogs";

/**
 * The sidebar's drop targets (shared/dnd): a playlist row takes songs,
 * albums and playlists dragged onto it; the Pinned section pins library
 * items; the "New playlist" row starts a playlist holding them.
 */

/** Apple keeps six pins. */
export const MAX_PINS = 6;

/** Whether the payload holds anything a playlist can take. */
export const addable = (p: DragPayload) => playlistItems(p.items).length > 0;

/**
 * Makes a library playlist row (`targetId`: its `data-drop`) a target for
 * adding; true while a drag is over it.
 */
export function usePlaylistDrop(
  targetId: string,
  playlist: { id: string; name: string; canEdit: boolean },
) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return useDropTarget(targetId, {
    accepts: (p) =>
      playlist.canEdit &&
      addable(p) &&
      !p.items.some((i) => i.kind === "playlist" && i.target.id === playlist.id),
    describe: () => {
      const name = playlist.name;
      return t`Add to “${name}”`;
    },
    drop: (p) =>
      addToPlaylistNow(
        qc,
        playlist,
        playlistItems(p.items),
        describeItems(p.items),
        () => void navigate({ to: "/playlist/$id", params: { id: playlist.id } }),
      ),
  });
}

/** The library items of a payload that can be pinned and are not yet ({id, name}). */
export function pinnable(p: DragPayload, pins: readonly Pin[]): { id: string; name: string }[] {
  const pinned = new Set(pins.map((x) => x.id));
  const out: { id: string; name: string }[] = [];
  for (const i of p.items) {
    if (i.kind === "station") return [];
    const id = libraryIdOf(i.target);
    if (!id) return [];
    if (!pinned.has(id)) out.push({ id, name: i.target.name });
  }
  return out.length && out.length + pins.length <= MAX_PINS ? out : [];
}

/** The Pinned section as a target: drops pin the dragged library items, in order. */
export function usePinDrop(pins: readonly Pin[]) {
  return useDropTarget("pins", {
    accepts: (p) => pinnable(p, pins).length > 0,
    describe: () => t`Pin to sidebar`,
    drop: (p) =>
      void (async () => {
        for (const it of pinnable(p, pins)) await pin(it.id, it.name);
      })(),
  });
}

/** The "New playlist" row as a target: opens the new-playlist dialog holding the items. */
export function useNewPlaylistDrop() {
  return useDropTarget("playlist:new", {
    accepts: addable,
    describe: () => t`New playlist`,
    drop: (p) =>
      showDialog({
        kind: "create",
        items: playlistItems(p.items),
        label: describeItems(p.items),
        folderId: "",
      }),
  });
}
