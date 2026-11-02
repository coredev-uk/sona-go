import type { Header } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import type { ReactNode, RefObject } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { MotionCover } from "@/shared/components/MotionCover";
import { catalogueLine } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";

import type { Disc } from "./album";

/**
 * The sleeve: a panel washed in the cover's colour (Go's pre-blurred
 * backdrop under the ground scrim, so text keeps its contrast) that holds
 * the cover and the record's fine print. It is the page's one coloured
 * surface; the listing beside it sits on the plain ground.
 */
export function Sleeve({
  header,
  className,
  children,
}: {
  header: Header;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cn("relative overflow-hidden rounded-lg", className)}>
      <Backdrop src={header.backdrop} color={header.colors[0]} tone="ground" />
      <div className="relative">{children}</div>
    </div>
  );
}

/** The cover at its station, moving where Apple has motion artwork and motion is allowed. */
export function AlbumCover({ art, motion, size }: { art: string; motion: string; size: number }) {
  return motion ? (
    <MotionCover motion={motion} art={art} size={size} alt="" eager />
  ) : (
    <Artwork src={art} size={size} alt="" eager />
  );
}

/**
 * The back of the sleeve: release day, label and ℗ line in meta type,
 * one fact a line.
 */
export function Credits({ lines }: { lines: readonly string[] }) {
  if (!lines.length) return null;
  return (
    <div className="space-y-1 text-meta text-ink-muted">
      {lines.map((l) => (
        <p key={l}>{l}</p>
      ))}
    </div>
  );
}

/**
 * A multi-disc album's discs, each with its catalogue line ("Disc 2 ·
 * 14 songs, 58 min"). Choosing one moves to its first song in the list,
 * keyboard focus included.
 */
export function DiscIndex({
  discs,
  list,
}: {
  discs: readonly Disc[];
  /** The element holding the album's track list. */
  list: RefObject<HTMLElement | null>;
}) {
  const { t } = useLingui();
  if (!discs.length) return null;
  const go = (first: number) => {
    const row = list.current?.querySelector<HTMLElement>(`[role="grid"] [data-row="${first}"]`);
    const main = row?.closest("main");
    if (!row || !main) return;
    // The disc's heading shares the first row's wrapper: bring it to the top.
    const block = row.parentElement ?? row;
    const top =
      block.getBoundingClientRect().top - main.getBoundingClientRect().top + main.scrollTop;
    main.scrollTo({ top: Math.max(0, top - 16) });
    row.focus({ preventScroll: true });
  };
  return (
    <ul aria-label={t`Discs`} className="-mx-2 space-y-0.5">
      {discs.map((d) => {
        const disc = d.number;
        const count = catalogueLine("", "", d.songs, d.durationMs);
        return (
          <li key={d.number}>
            <button
              type="button"
              onClick={() => go(d.first)}
              // From the keyboard the move happens on keydown, so WebKit draws the row's focus ring.
              onKeyDown={(e) => {
                if (e.key !== "Enter" && e.key !== " ") return;
                e.preventDefault();
                go(d.first);
              }}
              className="flex h-8 w-full min-w-0 items-center rounded-md px-2 text-left text-sub transition-colors duration-100 hover:bg-ink/6"
            >
              <span className="shrink-0 font-medium text-ink">{t`Disc ${disc}`}</span>
              <span className="truncate text-ink-muted">&nbsp;· {count}</span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}
