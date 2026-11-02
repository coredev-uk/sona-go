/**
 * The album page's layout for the content's width (docs/design.md, Album
 * page): the sleeve beside the listing from 860px, the cover larger from
 * 1280px; below 860px the sleeve heads the page with the title beside the
 * cover. Covers come in fixed sizes so Go serves them at the exact box.
 */
export interface AlbumLayout {
  wide: boolean;
  cover: number;
}

export function albumLayout(width: number): AlbumLayout {
  if (width >= 1280) return { wide: true, cover: 320 };
  if (width >= 860) return { wide: true, cover: 256 };
  return { wide: false, cover: 168 };
}
