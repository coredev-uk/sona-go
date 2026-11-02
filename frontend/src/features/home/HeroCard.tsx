import type { Item } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { useRef, useState } from "react";

import { ItemActionMenu } from "@/shared/actions/ActionMenu";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { openExternal } from "@/shared/items/cover";
import { actionItemOf, itemLink } from "@/shared/items/links";
import { canPlay, playItem } from "@/shared/items/play";
import { artUrl } from "@/shared/lib/art";
import { cn } from "@/shared/lib/utils";
import { useElementWidth } from "@/shared/lib/useCoverColumns";

import { useCatalogueLine } from "./catalogue-line";

/** Apple's hero card shape (music.apple.com crops the 4:1 banner to it). */
const HERO_RATIO = 1060 / 608;
/** The banner's share of the row; the name, lines and controls take the rest. */
const BANNER_SHARE = 0.4;

/**
 * An untitled hero shelf with one card (Apple's featured station on Home):
 * the banner beside its name, line and play control, filling the row
 * instead of leaving half of it empty.
 */
export function HeroCard({ item }: { item: Item }) {
  const { t } = useLingui();
  const ref = useRef<HTMLElement>(null);
  const width = useElementWidth(ref);
  const [failed, setFailed] = useState(false);
  const link = itemLink(item);
  const banner = item.wide.large || item.wide.medium;
  const src = banner || item.art.large || item.art.medium;
  const w = Math.round(width * BANNER_SHARE);
  const h = Math.round(w / HERO_RATIO);
  const meta = useCatalogueLine(item);
  const target = actionItemOf(item);
  const name = item.name;
  const opener = cn("rounded-sm outline-offset-4 hover:underline hover:underline-offset-4");
  return (
    <section ref={ref} aria-label={t`Featured`} className="mt-10 flex items-end gap-6">
      <div
        className="relative shrink-0 overflow-hidden rounded-sm bg-hover"
        style={{ width: w, height: h, backgroundColor: item.color || undefined }}
      >
        {width > 0 && !failed && src && (
          <img
            src={artUrl(src, { width: w, height: h }, banner ? "cover" : "contain")}
            alt=""
            loading="lazy"
            decoding="async"
            draggable={false}
            onError={() => setFailed(true)}
            className={`absolute inset-0 size-full rounded-[inherit] outline -outline-offset-1 outline-ink/6 ${banner ? "object-cover" : "object-contain"}`}
          />
        )}
      </div>
      <div className="min-w-0 flex-1 pb-1">
        <h2 className="line-clamp-2 text-page text-balance text-ink">
          {link ? (
            <Link {...link} draggable={false} className={opener}>
              {name}
            </Link>
          ) : (
            <button type="button" onClick={() => openExternal(item)} className={opener}>
              {name}
            </button>
          )}
        </h2>
        {item.subtitle && (
          <p className="mt-1.5 truncate text-heading font-medium text-ink">{item.subtitle}</p>
        )}
        {meta && <p className="mt-1 truncate text-meta text-ink-muted">{meta}</p>}
        <div className="mt-5 flex items-center gap-2">
          {canPlay(item) && <PlayButtons label={name} onPlay={() => playItem(item)} />}
          {target && <ItemActionMenu item={target} name={name} variant="header" />}
        </div>
      </div>
    </section>
  );
}
