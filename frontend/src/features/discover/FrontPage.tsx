import type { Item } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, Play } from "lucide-react";
import { useRef, useState, type KeyboardEvent, type MouseEvent } from "react";

import { ContextActionMenu } from "@/shared/actions/ActionMenu";
import { openExternal } from "@/shared/items/cover";
import { ItemMenu } from "@/shared/items/ItemMenu";
import { actionItemOf, itemLink, itemSubtitle } from "@/shared/items/links";
import { canPlay, playItem } from "@/shared/items/play";
import { artUrl } from "@/shared/lib/art";
import { GAP_X, useElementWidth } from "@/shared/lib/useCoverColumns";
import { Button } from "@/shared/ui/button";

/**
 * The hero card's shape on music.apple.com: its "flowcaseLockup" requests
 * the 4:1 subscriptionHero art cropped to 1060×608 ("sr").
 */
const HERO_RATIO = 1060 / 608;
/** The lead's caption under its art: 8px gap, a 22px title line, an 18px line. */
const CAPTION = 48;
/** The list's pager row: 8px gap and a 28px row. */
const PAGER = 36;
/** A list row's height under the lead, and the least beside it. */
const ROW = 72;
/** The list's thumbnail: the item's square cover (Apple's banners lose their text cropped this small). */
const THUMB = { width: 64, height: 64 };
/** The narrowest content width that sets the list beside the lead. */
const SPLIT_AT = 880;

interface Layout {
  /** The list sits beside the lead (else under it). */
  split: boolean;
  /** The lead's width, which its art is requested at. */
  lead: number;
  /** List rows per page, and per column. */
  perPage: number;
  perColumn: number;
  columns: number;
  rowHeight: number;
}

/**
 * Beside the lead (two thirds of the width), the list takes as many rows
 * as fit the lead's height, stretched so its pager ends level with the
 * lead's caption. Under the lead it runs in two columns of three.
 */
function layoutFor(width: number): Layout {
  if (width >= SPLIT_AT) {
    const lead = Math.round(((width - GAP_X) * 2) / 3);
    const room = lead / HERO_RATIO + CAPTION - PAGER;
    const perPage = Math.max(3, Math.floor(room / ROW));
    return {
      split: true,
      lead,
      perPage,
      perColumn: perPage,
      columns: 1,
      rowHeight: room / perPage,
    };
  }
  const columns = width >= 560 ? 2 : 1;
  return { split: false, lead: width, perPage: 3 * columns, perColumn: 3, columns, rowHeight: ROW };
}

/**
 * Discover's front page: Apple's featured cards as one lead, large, and the
 * rest as a list beside it (under it in a narrow window), paged so the
 * list never runs past the lead.
 */
export function FrontPage({ items }: { items: readonly Item[] }) {
  const { t } = useLingui();
  const ref = useRef<HTMLDivElement>(null);
  const width = useElementWidth(ref);
  const layout = layoutFor(width);
  const [page, setPage] = useState(0);
  const [lead, ...rest] = items;
  const pages = Math.max(1, Math.ceil(rest.length / layout.perPage));
  const at = Math.min(page, pages - 1);
  const shown = rest.slice(at * layout.perPage, (at + 1) * layout.perPage);
  const label = t`Featured`;
  const current = at + 1;
  // The pager wraps round rather than disabling a button, so focus never
  // drops from it to the page.
  const turn = (by: number) => setPage((at + by + pages) % pages);

  return (
    <section aria-label={label} className="mb-12">
      <div
        ref={ref}
        className="grid gap-y-6"
        style={{
          gridTemplateColumns: layout.split ? `${layout.lead}px minmax(0, 1fr)` : "minmax(0, 1fr)",
          columnGap: GAP_X,
        }}
      >
        {width > 0 && lead && <Lead item={lead} width={layout.lead} />}
        {width > 0 && rest.length > 0 && (
          <div className="flex min-w-0 flex-col">
            <ul
              aria-label={t`More featured`}
              className="-mx-2 grid grid-flow-col"
              style={{
                gridTemplateColumns: `repeat(${layout.columns}, minmax(0, 1fr))`,
                gridTemplateRows: `repeat(${Math.min(layout.perColumn, shown.length)}, ${layout.rowHeight}px)`,
                columnGap: GAP_X - 16,
              }}
            >
              {shown.map((item, i) => (
                <FeatureRow key={`${at}:${i}:${item.kind}:${item.id}`} item={item} />
              ))}
            </ul>
            {pages > 1 && (
              <div className="mt-auto flex h-7 shrink-0 items-center justify-end gap-1 pt-2">
                <span className="mr-1 text-meta text-ink-muted num" aria-live="polite">
                  {/* A new element per page: WebKitGTK announces only inserted nodes. */}
                  <span key={current}>
                    <Trans>
                      {current} of {pages}
                    </Trans>
                  </span>
                </span>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="size-7"
                  aria-label={t`Previous in ${label}`}
                  onClick={() => turn(-1)}
                >
                  <ChevronLeft />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="size-7"
                  aria-label={t`Next in ${label}`}
                  onClick={() => turn(1)}
                >
                  <ChevronRight />
                </Button>
              </div>
            )}
          </div>
        )}
      </div>
    </section>
  );
}

const openerClassName =
  "min-w-0 truncate rounded-sm text-left outline-offset-2 after:absolute after:inset-0 after:content-['']";

/** The item's page, or its web page for web-only items, as the card's whole-area opener. */
function Opener({ item, className }: { item: Item; className: string }) {
  const link = itemLink(item);
  return link ? (
    <Link {...link} draggable={false} className={`${openerClassName} ${className}`}>
      {item.name}
    </Link>
  ) : (
    <button
      type="button"
      className={`${openerClassName} ${className}`}
      onClick={() => openExternal(item)}
    >
      {item.name}
    </button>
  );
}

/**
 * A card's item menu at a point: a right-click, or Shift+F10 or the Menu
 * key on the card's focused control (its "more" button stays out of the
 * Tab order, as on every tile). Focus returns to that control.
 */
function useCardMenu(item: Item) {
  const target = actionItemOf(item);
  const [menu, setMenu] = useState<{ x: number; y: number; from?: HTMLElement } | null>(null);
  // React bubbles events out of the open menu's portal; only the card's own count.
  const own = (e: { currentTarget: HTMLElement; target: EventTarget }) =>
    e.currentTarget.contains(e.target as Node);
  const handlers = target && {
    onContextMenu: (e: MouseEvent<HTMLElement>) => {
      if (!own(e)) return;
      e.preventDefault();
      setMenu({ x: e.clientX, y: e.clientY });
    },
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => {
      if (!own(e) || (e.key !== "ContextMenu" && !(e.shiftKey && e.key === "F10"))) return;
      e.preventDefault();
      const from = e.target as HTMLElement;
      const r = from.getBoundingClientRect();
      setMenu({ x: r.left, y: r.bottom, from });
    },
  };
  const node = target && menu && (
    <ContextActionMenu
      item={target}
      name={item.name}
      at={menu}
      keyboard={!!menu.from}
      onClose={() => setMenu(null)}
      returnFocus={menu.from && (() => menu.from)}
    />
  );
  return { handlers, menu: node };
}

function Lead({ item, width }: { item: Item; width: number }) {
  const { t } = useLingui();
  // The wide banner cropped to the card, else the cover whole on its colour.
  const banner = item.wide.large || item.wide.medium;
  const src = banner || item.art.large || item.art.medium;
  const fit = banner ? "cover" : "contain";
  const [failed, setFailed] = useState(false);
  const { handlers, menu } = useCardMenu(item);
  const name = item.name;
  const subtitle = itemSubtitle(item);
  return (
    <div {...handlers} className="group/tile group/row relative flex min-w-0 flex-col">
      <div
        className="relative w-full overflow-hidden rounded-sm bg-hover"
        style={{ aspectRatio: HERO_RATIO, backgroundColor: item.color || undefined }}
      >
        {!failed && src && (
          <img
            src={artUrl(src, { width, height: width / HERO_RATIO }, fit)}
            alt=""
            decoding="async"
            draggable={false}
            onError={() => setFailed(true)}
            className={`absolute inset-0 size-full rounded-[inherit] outline -outline-offset-1 outline-ink/6 ${fit === "cover" ? "object-cover" : "object-contain"}`}
          />
        )}
        {canPlay(item) && (
          <button
            type="button"
            aria-label={t`Play ${name}`}
            onClick={() => playItem(item)}
            className="absolute bottom-4 left-4 z-10 grid size-12 place-items-center rounded-full bg-play text-on-play opacity-0 transition-opacity duration-100 group-focus-within/tile:opacity-100 group-hover/tile:opacity-100 focus-visible:opacity-100"
          >
            <Play fill="currentColor" strokeWidth={0} className="size-5 translate-x-px" />
          </button>
        )}
      </div>
      <div className="mt-2 flex h-[22px] min-w-0 items-center gap-1">
        <Opener
          item={item}
          className="flex-1 text-heading text-ink group-hover/tile:underline group-hover/tile:underline-offset-2"
        />
        <div className="relative z-10 -my-1 shrink-0">
          <ItemMenu item={item} />
        </div>
      </div>
      <span className="h-[18px] truncate text-sub text-ink-muted">{subtitle}</span>
      {menu}
    </div>
  );
}

function FeatureRow({ item }: { item: Item }) {
  const { t } = useLingui();
  const src = item.art.medium || item.art.large || item.wide.medium;
  const [failed, setFailed] = useState(false);
  const { handlers, menu } = useCardMenu(item);
  const name = item.name;
  const subtitle = itemSubtitle(item);
  return (
    <li
      {...handlers}
      className="group/row relative flex min-w-0 items-center gap-3 rounded-md px-2 transition-colors duration-100 hover:bg-hover"
    >
      <div
        className="relative shrink-0 overflow-hidden rounded-sm bg-hover"
        style={{ ...THUMB, backgroundColor: item.color || undefined }}
      >
        {!failed && src && (
          <img
            src={artUrl(src, THUMB)}
            alt=""
            width={THUMB.width}
            height={THUMB.height}
            loading="lazy"
            decoding="async"
            draggable={false}
            onError={() => setFailed(true)}
            className="size-full rounded-[inherit] object-cover outline -outline-offset-1 outline-ink/6"
          />
        )}
        {canPlay(item) && (
          <button
            type="button"
            aria-label={t`Play ${name}`}
            onClick={() => playItem(item)}
            className="absolute inset-0 z-10 m-auto grid size-8 place-items-center rounded-full bg-play text-on-play opacity-0 transition-opacity duration-100 group-focus-within/row:opacity-100 group-hover/row:opacity-100 focus-visible:opacity-100"
          >
            <Play fill="currentColor" strokeWidth={0} className="size-3.5 translate-x-px" />
          </button>
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <Opener item={item} className="text-ui font-medium text-ink" />
        {subtitle && <span className="truncate text-sub text-ink-muted">{subtitle}</span>}
      </div>
      <div className="relative z-10 shrink-0">
        <ItemMenu item={item} />
      </div>
      {menu}
    </li>
  );
}

/** The front page's frame while it loads: the lead and its list as flat bars at their real sizes. */
export function FrontPageSkeleton() {
  const ref = useRef<HTMLDivElement>(null);
  const width = useElementWidth(ref);
  const layout = layoutFor(width);
  return (
    <div
      ref={ref}
      aria-hidden
      className="mb-12 grid gap-y-6"
      style={{
        gridTemplateColumns: layout.split ? `${layout.lead}px minmax(0, 1fr)` : "minmax(0, 1fr)",
        columnGap: GAP_X,
      }}
    >
      {width > 0 && (
        <>
          <div>
            <div className="w-full rounded-sm bg-hover" style={{ aspectRatio: HERO_RATIO }} />
            <div className="mt-2 flex h-[22px] items-center">
              <div className="h-4 w-1/3 rounded-sm bg-hover" />
            </div>
            <div className="flex h-[18px] items-center">
              <div className="h-3 w-1/4 rounded-sm bg-hover" />
            </div>
          </div>
          <div
            className="grid grid-flow-col"
            style={{
              gridTemplateColumns: `repeat(${layout.columns}, minmax(0, 1fr))`,
              gridTemplateRows: `repeat(${layout.perColumn}, ${layout.rowHeight}px)`,
              columnGap: GAP_X,
            }}
          >
            {Array.from({ length: layout.perPage }, (_, i) => (
              <div key={i} className="flex items-center gap-3">
                <div className="shrink-0 rounded-sm bg-hover" style={THUMB} />
                <div className="flex-1">
                  <div className="h-3.5 w-3/4 rounded-sm bg-hover" />
                  <div className="mt-2 h-3 w-1/2 rounded-sm bg-hover" />
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
