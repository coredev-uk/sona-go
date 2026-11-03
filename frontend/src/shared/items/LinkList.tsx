import type { Item } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight } from "lucide-react";
import { useRef } from "react";

import { useElementWidth } from "@/shared/lib/useCoverColumns";

import { openExternal } from "./cover";
import { itemLink } from "./links";
import { LINK_GAP, linkGrid } from "./linkGrid";

/**
 * One destination as a button: a `--hover` tile, 44px at least, the name in
 * ink (two lines before it is cut), and an arrow out at the end of the one
 * that leaves Sona. In light a 1px `--line` border sets the tile off the ground (and
 * is the tile in forced colours, where fills go); in dark the fill alone does.
 */
const TILE =
  "flex min-h-11 w-full items-center justify-between gap-3 rounded-md border border-line bg-hover px-4 py-2 dark:border-transparent text-left text-ui font-medium text-ink transition-colors duration-[120ms] hover:bg-active active:bg-active";

/**
 * Links to editorial pages as a wall of buttons in even columns, in Apple's
 * order down each column (its genre lists are alphabetical; `linkGrid`);
 * web-only links say so. Apple splits a long list in two, the second
 * untitled: it continues the first, 24px under it.
 */
export function LinkList({ title, items }: { title: string; items: readonly Item[] }) {
  const { t } = useLingui();
  const ref = useRef<HTMLUListElement>(null);
  const { cols, rows } = linkGrid(useElementWidth(ref), items.length);
  return (
    <section aria-label={title || t`More links`} className={title ? "mt-10 first:mt-0" : "mt-6"}>
      {title && <h2 className="mb-3 text-heading text-ink">{title}</h2>}
      <ul
        ref={ref}
        className="grid grid-flow-col"
        style={{
          gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${rows}, auto)`,
          gap: LINK_GAP,
        }}
      >
        {items.map((item, i) => {
          const link = itemLink(item);
          return (
            <li key={`${i}:${item.name}`} className="flex">
              {link ? (
                <Link {...link} className={TILE}>
                  <span className="line-clamp-2 min-w-0">{item.name}</span>
                </Link>
              ) : (
                <button type="button" className={TILE} onClick={() => openExternal(item)}>
                  <span className="line-clamp-2 min-w-0">{item.name}</span>
                  <ArrowUpRight
                    aria-label={t`Opens in your browser`}
                    className="size-4 shrink-0 text-ink-muted"
                  />
                </button>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
