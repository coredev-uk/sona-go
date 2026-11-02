import type { Item, Section } from "@bindings/services";
import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { ChevronRight } from "lucide-react";
import { useId, useRef, useState } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { CoverRow } from "@/shared/components/CoverRow";
import { rowAspect, toCoverItem } from "@/shared/items/cover";
import { itemShape, type ItemLinkProps } from "@/shared/items/links";
import { useElementWidth } from "@/shared/lib/useCoverColumns";
import { cn } from "@/shared/lib/utils";

import { withoutSharedCaption } from "./plan";

/** Thumbnail edge and gap on an index line. */
const THUMB = 44;
const THUMB_GAP = 8;

interface ShelfIndexProps {
  sections: readonly Section[];
  moreFor: (section: Section) => ItemLinkProps | undefined;
}

/**
 * The rest of Apple's shelves (genres, "more like", fans also like), one
 * line each: the title, what it holds, and a strip of its covers. A line
 * opens its shelf in place. Twelve shelves take one screen instead of
 * four, and their full-size covers load only when asked for.
 */
export function ShelfIndex({ sections, moreFor }: ShelfIndexProps) {
  const heading = useId();
  return (
    <section aria-labelledby={heading} className="mt-12">
      <h2 id={heading} className="mb-2 text-heading text-ink">
        <Trans>More for you</Trans>
      </h2>
      <ul>
        {sections.map((s) => (
          <IndexLine key={s.id} section={s} more={moreFor(s)} />
        ))}
      </ul>
    </section>
  );
}

function IndexLine({ section, more }: { section: Section; more?: ItemLinkProps }) {
  const { t } = useLingui();
  const strip = useRef<HTMLSpanElement>(null);
  const width = useElementWidth(strip);
  const panel = useId();
  const [open, setOpen] = useState(false);
  const thumbs = Math.max(0, Math.floor((width + THUMB_GAP) / (THUMB + THUMB_GAP)));
  const items = withoutSharedCaption(section.items);
  return (
    <li>
      <h3>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={panel}
          onClick={() => setOpen((o) => !o)}
          className="flex h-16 w-full min-w-0 items-center gap-3 rounded-md px-2 text-left transition-colors duration-100 hover:bg-hover"
        >
          <ChevronRight
            aria-hidden
            className={cn(
              "size-4 shrink-0 text-ink-muted transition-transform duration-100",
              open && "rotate-90",
            )}
            strokeWidth={1.75}
          />
          <span className="w-[clamp(160px,28%,280px)] min-w-0 shrink-0">
            <span className="block truncate text-heading text-ink">{section.title}</span>
            <span className="sr-only">, </span>
            <span className="block truncate text-meta text-ink-muted num">
              {holds(section.items)}
            </span>
          </span>
          <span ref={strip} aria-hidden className="flex min-w-0 flex-1 gap-2 overflow-hidden">
            {/* Open, the shelf's own covers are right below: no strip. */}
            {(open ? [] : section.items.slice(0, thumbs)).map((it, i) => (
              <Artwork
                key={`${i}:${it.id}`}
                src={it.art.small || it.art.medium}
                size={THUMB}
                alt=""
                shape={itemShape(it)}
              />
            ))}
          </span>
        </button>
      </h3>
      <div id={panel} hidden={!open} className="px-2 pt-2 pb-8">
        {open && (
          <>
            <CoverRow
              items={items.map((it, i) => toCoverItem(it, i))}
              label={section.title || t`Items`}
              aspect={rowAspect(items)}
              rows={items.length}
            />
            {more && (
              <Link
                {...more}
                className="mt-4 inline-flex h-7 items-center gap-0.5 rounded-md px-1.5 text-sub text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink"
              >
                <Trans>See all</Trans>
                <ChevronRight aria-hidden className="size-3.5" strokeWidth={1.75} />
              </Link>
            )}
          </>
        )}
      </div>
    </li>
  );
}

/** What a shelf holds, by kind: "8 albums · 4 playlists". */
function holds(items: readonly Item[]) {
  const by = new Map<string, number>();
  for (const it of items) by.set(it.kind, (by.get(it.kind) ?? 0) + 1);
  const parts: string[] = [];
  for (const [kind, n] of by) {
    switch (kind) {
      case "album":
        parts.push(plural(n, { one: "# album", other: "# albums" }));
        break;
      case "playlist":
        parts.push(plural(n, { one: "# playlist", other: "# playlists" }));
        break;
      case "station":
        parts.push(plural(n, { one: "# station", other: "# stations" }));
        break;
      case "artist":
        parts.push(plural(n, { one: "# artist", other: "# artists" }));
        break;
      case "song":
        parts.push(plural(n, { one: "# song", other: "# songs" }));
        break;
    }
  }
  return parts.join(" · ");
}
