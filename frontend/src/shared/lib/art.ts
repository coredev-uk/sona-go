/**
 * Artwork sizing: the one place that turns the box an image renders in
 * into the pixel size requested from Go.
 *
 * Go's artwork URLs end in `<w>x<h>.jpg`, at the shape Apple serves the
 * artwork (square covers, 16:9 video frames, 2.4:1 and 4:1 editorial
 * banners; `<n>.jpg` in URLs saved by earlier versions means n×n). Go
 * serves any box whose long edge is 16–2048 by replacing that element, and
 * Apple crops to the box for most artwork, so a cover is requested at
 * exactly its box and never cropped twice.
 */

/** The largest long edge Go serves. */
const MAX_EDGE = 2048;
/** The smallest long edge Go serves. */
const MIN_EDGE = 16;
/** Device pixels per CSS pixel at most: beyond 2× nobody sees the difference. */
const MAX_DENSITY = 2;

/** A rendered box in CSS pixels. */
export interface ArtBox {
  width: number;
  height: number;
}

/**
 * How the image fills its box: `cover` fills it (Apple crops artwork of
 * another shape to the box), `contain` shows the whole image inside it at
 * its own aspect ratio (letterboxed banners).
 */
export type ArtFit = "cover" | "contain";

const sizePart = /\/(\d+)(?:x(\d+))?\.jpg(?=$|\?)/;

/** The width / height of the artwork behind a Go URL (1 when it names no size). */
export function artAspect(url: string | undefined): number {
  const m = url ? sizePart.exec(url) : null;
  if (!m) return 1;
  const w = Number(m[1]);
  const h = Number(m[2] ?? m[1]);
  return w > 0 && h > 0 ? w / h : 1;
}

/**
 * `url` sized for a `box` of CSS pixels at the screen's density (at most
 * MAX_DENSITY): the box itself for `cover`, the artwork's own shape inside
 * the box for `contain`. The long edge stays within what Go serves.
 */
export function artUrl(
  url: string | undefined,
  box: ArtBox,
  fit: ArtFit = "cover",
): string | undefined {
  if (!url) return undefined;
  const density = Math.min(MAX_DENSITY, Math.max(1, globalThis.devicePixelRatio || 1));
  let w = box.width;
  let h = box.height;
  if (fit === "contain") {
    const aspect = artAspect(url);
    if (aspect >= w / h) h = w / aspect;
    else w = h * aspect;
  }
  w *= density;
  h *= density;
  const long = Math.max(w, h);
  const scale = Math.min(MAX_EDGE, Math.max(MIN_EDGE, long)) / long;
  const pw = Math.max(1, Math.round(w * scale));
  const ph = Math.max(1, Math.round(h * scale));
  return url.replace(sizePart, `/${pw}x${ph}.jpg`);
}
