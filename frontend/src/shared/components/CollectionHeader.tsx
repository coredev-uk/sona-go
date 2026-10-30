import type { ReactNode } from "react";

import { Artwork, FluidArtwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
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
  actions?: ReactNode;
}

/**
 * The album, playlist and artist header: the cover, the title at 32/36,
 * the catalogue line, and the actions, on a wash of the cover's colour
 * (a Go pre-blurred backdrop under a 70% ground scrim). It bleeds to the
 * content edges.
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
  actions,
}: CollectionHeaderProps) {
  const narrow = useMediaQuery("(max-width: 960px)");
  return (
    <header className="relative -mx-8 -mt-7 mb-6 px-8 pt-8 pb-7 max-[960px]:-mx-6 max-[960px]:px-6">
      <Backdrop src={header?.backdrop} color={header?.colors[0]} scrim={70} tone="ground" />
      <div className="relative flex items-end gap-7">
        {aspect === "video" ? (
          <div className="shrink-0" style={{ width: narrow ? 240 : 356 }}>
            <FluidArtwork src={art} size={narrow ? 240 : 356} alt="" aspect="video" />
          </div>
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
          {actions && <div className="mt-5 flex flex-wrap items-center gap-2">{actions}</div>}
        </div>
      </div>
    </header>
  );
}
