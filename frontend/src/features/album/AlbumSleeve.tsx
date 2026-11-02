import type { Header } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import type { CSSProperties, ReactNode, Ref, RefObject } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { MotionCover } from "@/shared/components/MotionCover";
import { catalogueLine } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";

import type { Disc } from "./album";
import { sleeveVars, type SleeveColors } from "./palette";

/**
 * The sleeve: the page's one coloured surface, holding the cover and the
 * record's fine print. It is printed in the cover's own background colour
 * (`palette.ts`: one tone step off the ground, text checked to 4.5:1 on
 * it), its tokens redeclared so everything inside follows it. A cover
 * with no colour known (a library-only album) keeps the wash of Go's
 * pre-blurred backdrop under the ground scrim, with a 4% ink layer to
 * hold it one tone step off the ground. The listing beside it sits on the
 * plain ground.
 */
export function Sleeve({
  ref,
  header,
  colors,
  className,
  children,
}: {
  ref?: Ref<HTMLDivElement>;
  header: Header;
  colors: SleeveColors | undefined;
  className?: string;
  children: ReactNode;
}) {
  if (colors) {
    return (
      <div
        ref={ref}
        data-sleeve
        className={cn("relative rounded-lg bg-(--sleeve) text-ink", className)}
        style={sleeveVars(colors) as CSSProperties}
      >
        {children}
      </div>
    );
  }
  return (
    <div ref={ref} className={cn("relative overflow-hidden rounded-lg", className)}>
      <Backdrop src={header.backdrop} color={header.colors[0]} tone="ground" />
      <div aria-hidden className="absolute inset-0 bg-ink/4" />
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

/** One line of the fine print: what it is ("Label") and the record's entry. */
export interface Fact {
  label: string;
  value: string;
}

/**
 * The back of the sleeve: the record's facts in meta type, each a muted
 * name over its entry (released, first released, genres, who wrote or
 * composed it, label), then the ℗ line, which names itself.
 */
export function FinePrint({ facts, rights }: { facts: readonly Fact[]; rights: string }) {
  if (!facts.length && !rights) return null;
  return (
    <div className="text-meta">
      {facts.length > 0 && (
        <dl className="space-y-2">
          {facts.map((f) => (
            <div key={f.label}>
              <dt className="text-ink-muted">{f.label}</dt>
              <dd className="text-pretty text-ink">{f.value}</dd>
            </div>
          ))}
        </dl>
      )}
      {rights && (
        <p className={cn("text-pretty text-ink-muted", facts.length > 0 && "mt-3")}>{rights}</p>
      )}
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
