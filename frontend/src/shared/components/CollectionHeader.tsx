import type { ReactNode } from "react";

import { Artwork, FluidArtwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { MotionCover } from "@/shared/components/MotionCover";
import type { Header } from "@bindings/services";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";

interface CollectionHeaderProps {
  title: string;
  art?: string;
  shape?: "square" | "round";
  /** A music video's 16:9 frame instead of a square cover. */
  aspect?: "square" | "video";
  /** The line under the title: artist link, curator. */
  byline?: ReactNode;
  /** The catalogue line: `Album · 2008 · 10 songs, 42 min`. */
  meta?: string;
  /** Explicit and audio-quality labels, under the catalogue line. */
  badges?: ReactNode;
  description?: string;
  header?: Header;
  /** A state of the collection above the actions: an album's release countdown. */
  status?: ReactNode;
  actions?: ReactNode;
}

/**
 * The album, playlist, artist and station header: the cover, the title at
 * 32/36, the catalogue line, and the actions, on a wash of the cover's
 * colour (a Go pre-blurred backdrop under the theme's ground scrim). It
 * bleeds to the content edges. A cover with motion artwork
 * (`header.motion`) plays it, muted and looping, where motion is allowed
 * (an artist's clipped round).
 */
export function CollectionHeader({
  title,
  art,
  shape,
  aspect,
  byline,
  meta,
  badges,
  description,
  header,
  status,
  actions,
}: CollectionHeaderProps) {
  const narrow = useMediaQuery("(max-width: 960px)");
  return (
    <header className="relative -mx-8 -mt-7 mb-6 px-8 pt-8 pb-7 max-[960px]:-mx-6 max-[960px]:px-6">
      <Backdrop src={header?.backdrop} color={header?.colors[0]} tone="ground" />
      <div className="relative flex items-end gap-7">
        {aspect === "video" ? (
          <div className="shrink-0" style={{ width: narrow ? 240 : 356 }}>
            <FluidArtwork src={art} size={narrow ? 240 : 356} alt="" aspect="video" />
          </div>
        ) : header?.motion ? (
          <MotionCover
            motion={header.motion}
            art={art}
            size={narrow ? 136 : 200}
            videoSize={200}
            shape={shape}
            alt=""
            eager
          />
        ) : (
          <Artwork src={art} size={narrow ? 136 : 200} alt="" shape={shape} eager />
        )}
        <div className="min-w-0 flex-1 pb-1">
          <h1 className="line-clamp-2 text-display text-balance text-ink">{title}</h1>
          {byline && (
            <div className="mt-1.5 truncate text-heading font-medium text-ink">{byline}</div>
          )}
          {/* Proportional figures: tabular ones widen the commas, and one line needs no column alignment. */}
          {meta && <p className="mt-1 text-meta text-ink-muted">{meta}</p>}
          {badges && <div className="mt-2">{badges}</div>}
          {description && (
            <p className="mt-3 line-clamp-2 max-w-[72ch] text-sub text-ink-muted">{description}</p>
          )}
          {status && <div className="mt-4">{status}</div>}
          {actions && <div className="mt-5 flex flex-wrap items-center gap-2">{actions}</div>}
        </div>
      </div>
    </header>
  );
}
