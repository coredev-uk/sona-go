import { useEffect, useState } from "react";

import { cn } from "@/shared/lib/utils";

interface BackdropProps {
  /** Pre-blurred tiny JPEG from Go's artwork service. Never blurred in CSS. */
  src?: string;
  /** The cover's dominant colour, shown at once while the image decodes. */
  color?: string;
  /** Scrim strength over the image, as a percentage of the surface colour. */
  scrim?: number;
  /** The token the scrim is mixed from: the bar sits on --surface, headers on --ground. */
  tone?: "surface" | "ground";
  className?: string;
}

/**
 * A wash of the cover's colour behind a surface. The browser's bilinear
 * upscaling of the tiny image provides the softness; a solid scrim keeps
 * text contrast. Hidden entirely in reduce-effects mode (`[data-backdrop]`).
 * A new source is decoded before it fades in, so a track change never
 * flashes an empty frame.
 */
export function Backdrop({ src, color, scrim = 78, tone = "surface", className }: BackdropProps) {
  const [shown, setShown] = useState<string | undefined>(undefined);

  useEffect(() => {
    if (!src) return;
    let cancelled = false;
    const img = new Image();
    img.src = src;
    img
      .decode()
      .then(() => !cancelled && setShown(src))
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [src]);

  const visible = src && shown === src ? shown : undefined;
  if (!visible && !color) return null;
  return (
    <div
      data-backdrop
      aria-hidden
      className={cn("pointer-events-none absolute inset-0 overflow-hidden", className)}
    >
      {color && <div className="absolute inset-0" style={{ background: color }} />}
      {visible && (
        <img
          key={visible}
          src={visible}
          alt=""
          className="absolute inset-0 size-full scale-110 animate-in object-cover duration-300 ease-out fade-in"
        />
      )}
      <div
        className="absolute inset-0"
        style={{ background: `color-mix(in oklab, var(--${tone}) ${scrim}%, transparent)` }}
      />
    </div>
  );
}
