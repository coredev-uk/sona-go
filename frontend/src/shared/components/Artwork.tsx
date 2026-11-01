import { Music, User } from "lucide-react";
import { useEffect, useState } from "react";

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
}

const frameClassName =
  "relative shrink-0 overflow-hidden bg-hover after:pointer-events-none after:absolute after:inset-0 after:ring-1 after:ring-ink/6 after:ring-inset";
const rounding = (shape: "square" | "round") =>
  cn(shape === "round" ? "rounded-full after:rounded-full" : "rounded-sm after:rounded-sm");

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
export function Artwork({ src, size, alt, shape = "square", className, eager }: ArtworkProps) {
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
          className="size-full object-cover"
        />
      ) : (
        <>
          <Placeholder shape={shape} glyph={`${Math.max(12, size * 0.4)}px`} />
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
}: Omit<ArtworkProps, "className" | "eager"> & { aspect?: "square" | "video" }) {
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
          className="absolute inset-0 size-full object-cover"
        />
      ) : (
        <Placeholder shape={shape} glyph="40%" />
      )}
    </div>
  );
}
