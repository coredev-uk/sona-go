import type { ReactNode } from "react";

/**
 * The listing's head: the title at 32/36 (wrapping as far as it needs,
 * never moving the cover), the artist, the catalogue line, the badges,
 * Apple's one-line note, the album's state, then its actions.
 */
export function AlbumHead({
  title,
  byline,
  meta,
  badges,
  lede,
  status,
  actions,
}: {
  title: string;
  byline: ReactNode;
  /** The catalogue line: `Album · 2008 · 10 songs, 42 min · Rock`. */
  meta: string;
  badges: ReactNode;
  lede?: string;
  /** The countdown of an album not out yet, or how much of it the library holds. */
  status?: ReactNode;
  actions: ReactNode;
}) {
  return (
    <div className="min-w-0">
      <h1 className="text-display text-balance break-words text-ink">{title}</h1>
      <div className="mt-1.5 text-heading font-medium text-ink">{byline}</div>
      {/* Proportional figures: tabular ones widen the commas, and one line needs no column alignment. */}
      <p className="mt-1 text-meta text-ink-muted">{meta}</p>
      {badges && <div className="mt-2 empty:hidden">{badges}</div>}
      {lede && <p className="mt-3 max-w-[68ch] text-sub text-ink-muted">{lede}</p>}
      {status && <div className="mt-4">{status}</div>}
      <div className="mt-5 flex flex-wrap items-center gap-2">{actions}</div>
    </div>
  );
}
