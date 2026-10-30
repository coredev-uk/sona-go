import type { Item } from "@bindings/services";

import { ActionMenu } from "@/shared/actions/ActionMenu";

/** The item's "more" menu, for kinds with registered actions; null otherwise. */
export function ItemMenu({ item }: { item: Item }) {
  if (item.song) return <ActionMenu kind="song" target={item.song} name={item.name} />;
  if (item.album) return <ActionMenu kind="album" target={item.album} name={item.name} />;
  if (item.playlist) return <ActionMenu kind="playlist" target={item.playlist} name={item.name} />;
  if (item.station) return <ActionMenu kind="station" target={item.station} name={item.name} />;
  if (item.artist) return <ActionMenu kind="artist" target={item.artist} name={item.name} />;
  return null;
}
