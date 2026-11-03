import { Music, User } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";

import { artUrl } from "@/shared/lib/art";
import { cn } from "@/shared/lib/utils";

interface ArtworkProps {
  /** Any Go-served artwork URL; resized to the rendered size here. */
  src?: string;
  /** Rendered edge length in CSS pixels. */
  size: number;
  alt: string;
  /** Artists are round; everything else is a square record sleeve. */
  shape?: "square" | "round";
  className?: string;
  /** Eager-load covers that are above the fold on arrival (headers, now playing). */
  eager?: boolean;
  /** Drawn in place of the missing-art glyph while there is no picture (it fills the frame). */
  fallback?: ReactNode;
}

const frameClassName = "relative shrink-0 overflow-hidden bg-hover";
/**
 * The 1px line at 6% ink inside the cover's edge, so pale covers do not
 * dissolve into the ground. An outline on the image itself: WebKitGTK
 * repaints the visible page on every scroll step, and an inset ring on a
 * pseudo-element per tile cost about a quarter of a Home frame's paint;
 * the outline costs no measurable paint and, unlike a border, leaves the
 * image at the size it was decoded for.
 */
const edgeClassName = "rounded-[inherit] outline -outline-offset-1 outline-ink/6";
const rounding = (shape: "square" | "round") => (shape === "round" ? "rounded-full" : "rounded-sm");

/** The missing-art square: a 40%-ink outline glyph on --hover. */
function Placeholder({ shape, glyph }: { shape: "square" | "round"; glyph: string }) {
  const Glyph = shape === "round" ? User : Music;
  return (
    <span className="absolute inset-0 grid place-items-center">
      <Glyph
        aria-hidden
        className="text-ink/40"
        style={{ width: glyph, height: glyph }}
        strokeWidth={1.5}
      />
    </span>
  );
}

/** Waits before reloading a cover that failed (a new playlist's cover renders for a while). */
const RETRY_MS = [5_000, 30_000];

/**
 * Loads `url`, showing the placeholder (never the broken-image glyph)
 * while it fails. A failed load is retried after each RETRY_MS delay with
 * a fresh `?retry=n` URL (Go marks failures no-store), then left alone.
 */
function useImage(url: string | undefined) {
  const [state, setState] = useState<{ url?: string; fails: number; waiting: boolean }>({
    fails: 0,
    waiting: false,
  });
  const s = state.url === url ? state : { url, fails: 0, waiting: false };
  useEffect(() => {
    if (!s.waiting) return;
    const t = setTimeout(
      () => setState({ url, fails: s.fails, waiting: false }),
      RETRY_MS[s.fails - 1],
    );
    return () => clearTimeout(t);
  }, [url, s.waiting, s.fails]);
  return {
    src: url && s.fails ? `${url}?retry=${s.fails}` : url,
    ok: !!url && !s.waiting && s.fails <= RETRY_MS.length,
    onError: () => setState({ url, fails: s.fails + 1, waiting: s.fails < RETRY_MS.length }),
  };
}

/** Square cover art at an exact size, lazy-loaded, with a placeholder when missing. */
export function Artwork({
  src,
  size,
  alt,
  shape = "square",
  className,
  eager,
  fallback,
}: ArtworkProps) {
  const url = artUrl(src, { width: size, height: size });
  const img = useImage(url);
  return (
    <div
      className={cn(frameClassName, rounding(shape), className)}
      style={{ width: size, height: size }}
    >
      {img.ok ? (
        <img
          src={img.src}
          alt={alt}
          width={size}
          height={size}
          loading={eager ? "eager" : "lazy"}
          decoding="async"
          draggable={false}
          onError={img.onError}
          className={cn("size-full object-cover", edgeClassName)}
        />
      ) : (
        <>
          {fallback ?? <Placeholder shape={shape} glyph={`${Math.max(12, size * 0.4)}px`} />}
          {alt && <span className="sr-only">{alt}</span>}
        </>
      )}
    </div>
  );
}

/**
 * A cover that fills its container's width (grid tiles). `size` is the
 * largest width the tile renders at, so the request stays exact. Music
 * video frames are 16:9 (`aspect="video"`), requested at that box.
 */
export function FluidArtwork({
  src,
  size,
  alt,
  shape = "square",
  aspect = "square",
}: Omit<ArtworkProps, "className" | "eager" | "fallback"> & { aspect?: "square" | "video" }) {
  const url = artUrl(src, { width: size, height: aspect === "video" ? (size * 9) / 16 : size });
  const img = useImage(url);
  return (
    <div
      className={cn(
        frameClassName,
        rounding(shape),
        aspect === "video" ? "aspect-video" : "aspect-square",
        "w-full",
      )}
    >
      {img.ok ? (
        <img
          src={img.src}
          alt={alt}
          loading="lazy"
          decoding="async"
          draggable={false}
          onError={img.onError}
          className={cn("absolute inset-0 size-full object-cover", edgeClassName)}
        />
      ) : (
        <Placeholder shape={shape} glyph="40%" />
      )}
    </div>
  );
}
