import type { Item } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, Pause, Play } from "lucide-react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
} from "react";

import { ContextActionMenu } from "@/shared/actions/ActionMenu";
import { openExternal } from "@/shared/items/cover";
import { ItemMenu } from "@/shared/items/ItemMenu";
import { actionItemOf, itemLink, itemSubtitle } from "@/shared/items/links";
import { canPlay, playItem } from "@/shared/items/play";
import { artUrl } from "@/shared/lib/art";
import { GAP_X, useElementWidth } from "@/shared/lib/useCoverColumns";
import { Button } from "@/shared/ui/button";

import { useLeadRotation } from "./useLeadRotation";

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
 * Discover's front page: Apple's featured cards, one of them the lead,
 * large, and all of them as a list beside it (under it in a narrow window),
 * paged so the list never runs past the lead. The lead moves on by itself
 * (useLeadRotation) unless the user pauses it or asks for reduced motion;
 * choosing a row makes it the lead, and the list shows the page the lead
 * is on.
 */
export function FrontPage({ items }: { items: readonly Item[] }) {
  const { t } = useLingui();
  const sectionRef = useRef<HTMLElement>(null);
  const ref = useRef<HTMLDivElement>(null);
  const width = useElementWidth(ref);
  const layout = layoutFor(width);
  // `from` is the lead on its way out while the new one fades in.
  const [view, setView] = useState<{ at: number; from: number | null }>({ at: 0, from: null });
  const [turned, setTurned] = useState<{ at: number; page: number } | null>(null);
  const [said, setSaid] = useState({ n: 0, text: "" });
  const count = items.length;
  const at = Math.min(view.at, count - 1);
  const lead = items[at];
  const choose = useCallback(
    (to: number) => setView((v) => (to === v.at ? v : { at: to, from: v.at })),
    [],
  );
  const advance = useCallback(() => choose((at + 1) % count), [choose, at, count]);
  const rotation = useLeadRotation(sectionRef, { count, at, advance });
  // The next lead's picture is fetched and decoded while this one is read,
  // so it fades in as a picture rather than as an empty box.
  const upcoming = rotation.available && width > 0 ? items[(at + 1) % count] : undefined;
  useEffect(() => {
    const { src } = upcoming ? leadArt(upcoming, layout.lead) : { src: "" };
    if (!src) return;
    const img = new Image();
    img.src = src;
    // jsdom has no decode().
    void img.decode?.().catch(() => {});
  }, [upcoming, layout.lead]);
  const pages = Math.max(1, Math.ceil(count / layout.perPage));
  // The list shows the lead's page until the user turns it, and returns
  // to it when the lead changes.
  const page = Math.min(
    turned?.at === at ? turned.page : Math.floor(at / layout.perPage),
    pages - 1,
  );
  const shown = items.slice(page * layout.perPage, (page + 1) * layout.perPage);
  const label = t`Featured`;
  const current = page + 1;
  // Said only for what the user did: the lead and the list's page change
  // by themselves, and a live region on those would speak every few seconds.
  const say = (text: string) => setSaid((s) => ({ n: s.n + 1, text }));
  // The pager wraps round rather than disabling a button, so focus never
  // drops from it to the page.
  const turn = (by: number) => {
    const to = (page + by + pages) % pages;
    setTurned({ at, page: to });
    const current = to + 1;
    say(t`${current} of ${pages}`);
  };
  const toggle = () => {
    say(rotation.paused ? t`Rotation playing` : t`Rotation paused`);
    rotation.toggle();
  };

  return (
    <section ref={sectionRef} aria-label={label} className="mb-12">
      <div
        ref={ref}
        className="grid gap-y-6"
        style={{
          gridTemplateColumns: layout.split ? `${layout.lead}px minmax(0, 1fr)` : "minmax(0, 1fr)",
          columnGap: GAP_X,
        }}
      >
        {width > 0 && lead && (
          <div className="grid min-w-0">
            {view.from !== null && items[view.from] && (
              <div
                key={leadKey(items[view.from], view.from)}
                inert
                aria-hidden
                className="lead-out pointer-events-none col-start-1 row-start-1 min-w-0"
                onAnimationEnd={(e) => {
                  if (e.target === e.currentTarget) setView((v) => ({ ...v, from: null }));
                }}
              >
                <Lead item={items[view.from]} width={layout.lead} />
              </div>
            )}
            <div
              key={leadKey(lead, at)}
              className={`col-start-1 row-start-1 min-w-0 ${view.from === null ? "" : "lead-in"}`}
            >
              <Lead item={lead} width={layout.lead} />
            </div>
          </div>
        )}
        {width > 0 && count > 1 && (
          <div className="flex min-w-0 flex-col">
            <ul
              aria-label={t`Featured cards`}
              className="-mx-2 grid grid-flow-col"
              style={{
                gridTemplateColumns: `repeat(${layout.columns}, minmax(0, 1fr))`,
                gridTemplateRows: `repeat(${Math.min(layout.perColumn, shown.length)}, ${layout.rowHeight}px)`,
                columnGap: GAP_X - 16,
              }}
            >
              {shown.map((item, i) => {
                const index = page * layout.perPage + i;
                return (
                  <FeatureRow
                    key={`${page}:${i}:${item.kind}:${item.id}`}
                    item={item}
                    current={index === at}
                    onChoose={() => choose(index)}
                  />
                );
              })}
            </ul>
            {(pages > 1 || rotation.available) && (
              <div className="mt-auto flex h-7 shrink-0 items-center justify-end gap-1 pt-2">
                {rotation.available && (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="mr-2 size-7"
                    aria-label={
                      rotation.paused ? t`Play featured rotation` : t`Pause featured rotation`
                    }
                    onClick={toggle}
                  >
                    {rotation.paused ? (
                      <Play fill="currentColor" strokeWidth={0} />
                    ) : (
                      <Pause fill="currentColor" strokeWidth={0} />
                    )}
                  </Button>
                )}
                {pages > 1 && (
                  <>
                    <span className="mr-1 text-meta text-ink-muted num">
                      <Trans>
                        {current} of {pages}
                      </Trans>
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
                  </>
                )}
              </div>
            )}
          </div>
        )}
      </div>
      {/* A new element per message: WebKitGTK announces only inserted nodes. */}
      <span className="sr-only" aria-live="polite">
        <span key={said.n}>{said.text}</span>
      </span>
    </section>
  );
}

/** The lead's key: its own card, so a card that changes place keeps its state. */
function leadKey(item: Item, index: number) {
  return `${index}:${item.kind}:${item.id}`;
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

/** The lead's picture at its width: the wide banner cropped to the card, else the cover whole on its colour. */
function leadArt(item: Item, width: number) {
  const banner = item.wide.large || item.wide.medium;
  const from = banner || item.art.large || item.art.medium;
  const fit = banner ? "cover" : "contain";
  return {
    src: from ? artUrl(from, { width, height: width / HERO_RATIO }, fit) : "",
    fit,
  } as const;
}

function Lead({ item, width }: { item: Item; width: number }) {
  const { t } = useLingui();
  const { src, fit } = leadArt(item, width);
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
            src={src}
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

/** A list row: choosing it makes its card the lead (the lead's own title opens the card). */
function FeatureRow({
  item,
  current,
  onChoose,
}: {
  item: Item;
  /** The row is the lead now. */
  current: boolean;
  onChoose: () => void;
}) {
  const { t } = useLingui();
  const src = item.art.medium || item.art.large || item.wide.medium;
  const [failed, setFailed] = useState(false);
  const { handlers, menu } = useCardMenu(item);
  const name = item.name;
  const subtitle = itemSubtitle(item);
  return (
    <li
      {...handlers}
      className={`group/row relative flex min-w-0 items-center gap-3 rounded-md px-2 transition-colors duration-100 ${current ? "bg-active" : "hover:bg-hover"}`}
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
        <button
          type="button"
          aria-current={current || undefined}
          className={`${openerClassName} text-ui font-medium text-ink`}
          onClick={onChoose}
        >
          {name}
        </button>
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
