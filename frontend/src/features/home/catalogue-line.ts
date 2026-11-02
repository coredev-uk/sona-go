import type { Item } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";

/** The catalogue line of a featured item: what it is, then Apple's reason ("Album · New Release"). */
export function useCatalogueLine(item: Item) {
  const { t } = useLingui();
  const kinds: Record<string, string> = {
    album: t`Album`,
    playlist: t`Playlist`,
    station: t`Station`,
    artist: t`Artist`,
    song: t`Song`,
    "music-video": t`Music video`,
  };
  return [kinds[item.kind], item.caption].filter(Boolean).join(" · ");
}
