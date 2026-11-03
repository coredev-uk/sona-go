import type { Item } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, Play } from "lucide-react";
import { useRef, useState } from "react";

import { GAP_X, useCoverColumns } from "@/shared/lib/useCoverColumns";
import { artAspect, artUrl } from "@/shared/lib/art";
import { Button } from "@/shared/ui/button";

import { openWithoutPage } from "./cover";
import { ItemMenu } from "./ItemMenu";
import { itemLink, itemSubtitle } from "./links";
import { canPlay, playItem } from "./play";

interface WideRowProps {
  title?: string;
  items: readonly Item[];
  /** Smallest card width: 420 for heroes, 240 for category bricks. */
  min: number;
  /**
   * The artwork frame, width / height. With `crop` it is every frame's
   * shape; otherwise it is the tallest frame (2 for bricks), and a row
   * whose banners are all wider gets their shape instead.
   */
  ratio: number;
  /**
   * Crop every banner to `ratio`, as Apple's hero shelf does: its 4:1 art
   * keeps what matters in the centre, and Apple's CDN crops it to the box.
   */
  crop?: boolean;
  onOpen?: (item: Item) => void;
}

/**
 * Wide editorial cards: the banner (never under text), the title and line
 * below. Every frame in a row has one shape. A cropping row (heroes) fills
 * it with each banner cropped to it. Otherwise banners sit whole on their
 * own background colour in the tallest of the row's banner shapes within
 * `ratio`, so a row of 4:1 bricks has no empty bands and a mixed row
 * letterboxes only the wider ones. As many as fit; arrows page through the
 * rest.
 */
export function WideRow({ title, items, min, ratio, crop = false, onOpen }: WideRowProps) {
  const { t } = useLingui();
  const ref = useRef<HTMLDivElement>(null);
  const { cols, tile } = useCoverColumns(ref, min);
  const aspects = items.flatMap((i) => {
    const wide = i.wide.large || i.wide.medium;
    return wide ? [artAspect(wide)] : [];
  });
  const frame = crop || !aspects.length ? ratio : Math.max(ratio, Math.min(...aspects));
  const [page, setPage] = useState(0);
  const pages = Math.max(1, Math.ceil(items.length / cols));
  const at = Math.min(page, pages - 1);
  const shown = items.slice(at * cols, at * cols + cols);
  // Apple leaves heroes untitled; a paged one still gets a heading.
  const label = title || t`Featured`;
  const current = at + 1;

  return (
    <section aria-label={label} className="mt-10 first:mt-0">
      {(title || pages > 1) && (
        <div className="mb-3 flex min-h-7 items-center justify-between gap-4">
          <h2 className="truncate text-heading text-ink">{label}</h2>
          {pages > 1 && (
            <div className="flex shrink-0 items-center gap-1">
              <span className="mr-1 text-meta text-ink-faint num" aria-live="polite">
                <Trans>
                  {current} of {pages}
                </Trans>
              </span>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t`Previous in ${label}`}
                disabled={at === 0}
                onClick={() => setPage(at - 1)}
              >
                <ChevronLeft />
              </Button>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t`Next in ${label}`}
                disabled={at >= pages - 1}
                onClick={() => setPage(at + 1)}
              >
                <ChevronRight />
              </Button>
            </div>
          )}
        </div>
      )}
      <div
        ref={ref}
        role="list"
        aria-label={label}
        className="grid"
        style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, columnGap: GAP_X }}
      >
        {shown.map((item, i) => (
          <div role="listitem" key={`${at}:${i}:${item.kind}:${item.id}`}>
            <WideCard item={item} width={tile} ratio={frame} crop={crop} onOpen={onOpen} />
          </div>
        ))}
      </div>
    </section>
  );
}

function WideCard({
  item,
  width,
  ratio,
  crop,
  onOpen,
}: {
  item: Item;
  width: number;
  ratio: number;
  crop: boolean;
  onOpen?: (item: Item) => void;
}) {
  const { t } = useLingui();
  const link = itemLink(item);
  const banner = item.wide.large || item.wide.medium;
  // A card without a banner shows its cover, whole, on the same colour.
  const src = banner || item.art.large || item.art.medium;
  const fit = crop && banner ? "cover" : "contain";
  const [failed, setFailed] = useState(false);
  const openerClassName =
    "min-w-0 flex-1 truncate rounded-sm text-left text-ui font-medium text-ink outline-offset-4 after:absolute after:inset-0 after:content-[''] group-hover/tile:underline group-hover/tile:underline-offset-2";
  const name = item.name;
  const subtitle = itemSubtitle(item);
  return (
    <div className="group/tile group/row relative flex min-w-0 flex-col">
      <div
        className="relative w-full overflow-hidden rounded-sm bg-hover"
        style={{ aspectRatio: ratio, backgroundColor: item.color || undefined }}
      >
        {!failed && src && (
          <img
            src={artUrl(src, { width, height: width / ratio }, fit)}
            alt=""
            loading="lazy"
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
            className="absolute bottom-3 left-3 z-10 grid size-10 place-items-center rounded-full bg-play text-on-play opacity-0 transition-opacity duration-100 group-focus-within/tile:opacity-100 group-hover/tile:opacity-100 focus-visible:opacity-100"
          >
            <Play fill="currentColor" strokeWidth={0} className="size-[18px] translate-x-px" />
          </button>
        )}
      </div>
      <div className="mt-2 flex h-5 min-w-0 items-center gap-1">
        {link ? (
          <Link
            {...link}
            onClick={onOpen && (() => onOpen(item))}
            draggable={false}
            className={openerClassName}
          >
            {item.name}
          </Link>
        ) : (
          <button
            type="button"
            className={openerClassName}
            onClick={() => {
              onOpen?.(item);
              openWithoutPage(item);
            }}
          >
            {item.name}
          </button>
        )}
        <div className="relative z-10 -my-1 shrink-0">
          <ItemMenu item={item} />
        </div>
      </div>
      {subtitle ? (
        <span className="line-clamp-2 text-sub text-ink-muted">{subtitle}</span>
      ) : (
        <span className="h-[18px]" />
      )}
    </div>
  );
}
