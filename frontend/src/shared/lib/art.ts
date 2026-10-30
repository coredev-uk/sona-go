/**
 * A Go artwork URL at `cssSize` × 2 device pixels. Go serves any size from
 * 16 to 2048 by replacing the final `<size>.jpg` path element, so every
 * image is requested at exactly the size it renders.
 */
export function artAt(url: string | undefined, cssSize: number): string | undefined {
  if (!url) return undefined;
  const px = Math.min(2048, Math.max(16, Math.round(cssSize * 2)));
  return url.replace(/\/\d+\.jpg(?=$|\?)/, `/${px}.jpg`);
}
