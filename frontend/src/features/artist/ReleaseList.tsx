import type { Album } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { memo, useRef, type KeyboardEvent } from "react";

import { ItemActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { ExplicitMark } from "@/shared/components/Badges";
import { useWithheld } from "@/shared/lib/content-policy";
import { yearOf } from "@/shared/lib/format";
import { releaseBadge } from "@/shared/lib/release";
import { cn } from "@/shared/lib/utils";
import { SelectionContext, useRowSelected } from "@/shared/selection/store";
import { useItemList } from "@/shared/selection/useItemList";

import { releaseTitle } from "./artist";

/** A row's cover, and the row's height: the cover plus 6px above and below. */
const COVER = 40;
export const RELEASE_ROW = COVER + 12;

/**
 * One release: its cover, its title and its line (the year and whether it
 * was a single or an EP; `by: "artist"`: whose release it is, then the
 * year), linking to the album.
 */
const ReleaseRow = memo(function ReleaseRow({
  album,
  index,
  by,
}: {
  album: Album | undefined;
  index: number;
  by: "year" | "artist";
}) {
  const { t } = useLingui();
  const selected = useRowSelected(album?.id);
  const withheld = useWithheld(album?.explicit);
  if (!album) {
    return (
      <div role="row" aria-busy className="flex h-[52px] items-center gap-3 px-2">
        <span className="size-10 shrink-0 rounded-sm bg-hover" />
        <span className="flex flex-1 flex-col gap-1.5">
          <span className="h-3.5 w-3/5 rounded-sm bg-hover" />
          <span className="h-3 w-1/4 rounded-sm bg-hover" />
        </span>
      </div>
    );
  }
  const { title, kind } = releaseTitle(album.name);
  const when = releaseBadge(album) ?? yearOf(album.releaseDate);
  const line = (
    by === "artist"
      ? [album.artistName, when]
      : [when, kind === "single" ? t`Single` : kind === "ep" ? t`EP` : ""]
  )
    .filter(Boolean)
    .join(" · ");
  return (
    <div role="row">
      <div
        role="gridcell"
        data-row={index}
        aria-selected={selected}
        title={withheld ? t`Explicit content is restricted on this account` : undefined}
        className={cn(
          "group/row relative isolate flex h-[52px] items-center gap-3 rounded-md px-2 transition-colors duration-100 hover:bg-hover",
          selected && "bg-ink/12 hover:bg-ink/12",
          withheld && "opacity-50",
        )}
      >
        <Artwork src={album.art.medium} size={COVER} alt="" />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="flex min-w-0 items-center gap-1.5">
            {/* The title is the row's link; its ::after covers the row, under the menu. */}
            <Link
              to="/album/$id"
              params={{ id: album.id }}
              draggable={false}
              data-opener
              className="min-w-0 truncate rounded-sm text-ui font-medium text-ink outline-offset-2 group-hover/row:underline group-hover/row:underline-offset-2 after:absolute after:inset-0 after:rounded-md after:content-['']"
            >
              {title}
            </Link>
            {album.explicit && <ExplicitMark />}
          </span>
          <span className="truncate text-sub text-ink-muted num">{line}</span>
        </span>
        <span className="relative z-10 shrink-0">
          <ItemActionMenu
            item={{ kind: "album", target: album }}
            name={album.name}
            selectionKey={album.id}
          />
        </span>
      </div>
    </div>
  );
});

/**
 * Releases as a compact list, the label's back catalogue: 40px covers, the
 * title, then the year and whether it was a single or an EP (or, `by:
 * "artist"`, whose it is). `columns` flows the list down the first column,
 * then the next. Rows select, open their menu on right-click or Shift+F10
 * and drag like cover tiles; ↑ and ↓ move between them. `undefined` items
 * are rows still loading. Covers line up with the heading above; the hover
 * fill reaches 8px past them.
 */
export function ReleaseList({
  albums,
  label,
  columns = 1,
  by = "year",
}: {
  albums: readonly (Album | undefined)[];
  label: string;
  columns?: number;
  by?: "year" | "artist";
}) {
  const ref = useRef<HTMLDivElement>(null);
  const list = useItemList({
    root: ref,
    keys: albums.map((a) => a?.id),
    itemAt: (i) => {
      const a = albums[i];
      return a && { kind: "album", target: a };
    },
    plainClick: "open",
  });
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const cell = e.target instanceof Element ? e.target.closest<HTMLElement>("[data-row]") : null;
    if (!cell || !(e.target instanceof HTMLElement) || !e.target.hasAttribute("data-opener"))
      return;
    const at = Number(cell.dataset.row);
    if (list.selectionKey(e, at)) return;
    const step = { ArrowUp: -1, ArrowDown: 1 }[e.key];
    if (step === undefined || e.altKey) return;
    const to = at + step;
    const next = ref.current?.querySelector<HTMLElement>(`[data-row="${to}"] [data-opener]`);
    if (!next) return;
    e.preventDefault();
    next.focus();
    list.arrowTo(e, to);
  };
  const rows = Math.ceil(albums.length / columns);
  return (
    <SelectionContext value={list.scope}>
      <div
        ref={ref}
        role="grid"
        aria-label={label}
        aria-multiselectable
        onKeyDown={onKeyDown}
        className="-mx-2 grid grid-flow-col gap-x-6"
        style={{
          gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${rows}, ${RELEASE_ROW}px)`,
        }}
        {...list.rootProps}
      >
        {albums.map((a, i) => (
          <ReleaseRow key={a?.id ?? `loading-${i}`} album={a} index={i} by={by} />
        ))}
        {list.menu}
      </div>
    </SelectionContext>
  );
}
