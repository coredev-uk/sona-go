import type { Item, Section } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { ChevronDown, WifiOff } from "lucide-react";
import { useId, useRef, useState, type ReactNode } from "react";

import { ItemActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { ShelfToggle } from "@/shared/components/Shelf";
import { openExternal } from "@/shared/items/cover";
import { actionItemOf, itemLink, itemShape } from "@/shared/items/links";
import { canPlay, playItem } from "@/shared/items/play";
import { openAccountDialog } from "@/shared/lib/account";
import { useWithheld } from "@/shared/lib/content-policy";
import { useElementWidth } from "@/shared/lib/useCoverColumns";
import { cn } from "@/shared/lib/utils";
import { useItemList } from "@/shared/selection/useItemList";
import { Button } from "@/shared/ui/button";

import { useCatalogueLine } from "./catalogue-line";
import { ItemRows } from "./ItemRows";
import { withoutSharedCaption } from "./plan";

/** From this width the lead and the other picks sit side by side; below it they stack. */
const SIDE_BY_SIDE = 900;
/** Rows of picks beside (or under) the lead. */
const SLATE_ROWS = 4;
const SIDE_BY_SIDE_COLUMNS = "minmax(0, 5fr) minmax(0, 6fr)";

/**
 * The top of Home: the page header and the lead, on a wash of the lead's
 * cover (Go's pre-blurred backdrop under the ground scrim), full bleed like
 * a collection header. Without a lead it is the plain header.
 */
export function LeadBand({ lead, children }: { lead?: Item; children: ReactNode }) {
  if (!lead) return <div>{children}</div>;
  const art = lead.art.medium || lead.art.large;
  return (
    <div className="relative -mx-8 -mt-7 px-8 pt-7 pb-6 max-[960px]:-mx-6 max-[960px]:px-6">
      <Backdrop
        src={art?.replace(/\/\d+(?:x\d+)?\.jpg(?=$|\?)/, "/backdrop.jpg")}
        color={lead.color || undefined}
        tone="ground"
      />
      <div className="relative">{children}</div>
    </div>
  );
}

/**
 * The first "feature" shelf (Top Picks) set as the lead: its first pick
 * large with what it is, Apple's reason and its play control, and every
 * other pick as a row beside it, so the first screen answers "what shall I
 * play?" without scrolling.
 */
export function HomeLead({ section }: { section: Section }) {
  const { t } = useLingui();
  const ref = useRef<HTMLElement>(null);
  const wide = useElementWidth(ref) >= SIDE_BY_SIDE;
  const heading = useId();
  const [cols, setCols] = useState(0);
  const [open, setOpen] = useState(false);
  const [first, ...rest] = withoutSharedCaption(section.items);
  const hidden = cols > 0 && rest.length > cols * SLATE_ROWS;
  const count = section.items.length;
  const title = section.title || t`Top picks`;
  return (
    <section ref={ref} aria-labelledby={heading}>
      <div className="mb-4 flex min-h-7 items-center justify-between gap-4">
        <h2 id={heading} className="truncate text-heading text-ink">
          {title}
        </h2>
        {(hidden || open) && (
          <ShelfToggle open={open} onToggle={() => setOpen((o) => !o)}>
            {open ? <Trans>Show fewer</Trans> : <Trans>Show all {count}</Trans>}
            <ChevronDown
              aria-hidden
              className={cn("size-3.5 transition-transform duration-100", open && "rotate-180")}
              strokeWidth={1.75}
            />
          </ShelfToggle>
        )}
      </div>
      <div
        className={cn("grid gap-x-8 gap-y-6", wide ? "items-end" : "grid-cols-1")}
        style={wide ? { gridTemplateColumns: SIDE_BY_SIDE_COLUMNS } : undefined}
      >
        {first && <LeadPick item={first} cover={wide ? 232 : 168} />}
        {rest.length > 0 && (
          <ItemRows
            items={rest}
            label={title}
            rows={SLATE_ROWS}
            min={248}
            open={open}
            onColumns={setCols}
            onBackdrop
          />
        )}
      </div>
    </section>
  );
}

/** The lead pick: its cover, name, artist or curator, the catalogue line and its controls. */
function LeadPick({ item, cover }: { item: Item; cover: number }) {
  const { t } = useLingui();
  const ref = useRef<HTMLDivElement>(null);
  const withheld = useWithheld(item.explicit);
  const target = actionItemOf(item);
  const link = itemLink(item);
  // Right-click, Shift+F10 and drag work on the lead as on any tile.
  const list = useItemList({
    root: ref,
    keys: [target ? `lead:${item.kind}:${item.id}` : undefined],
    itemAt: () => target,
    plainClick: "open",
  });
  const meta = useCatalogueLine(item);
  const name = item.name;
  const art = (
    <Artwork
      src={item.art.large || item.art.medium}
      size={cover}
      alt=""
      shape={itemShape(item)}
      eager
    />
  );
  const opener = cn("rounded-sm outline-offset-4 hover:underline hover:underline-offset-4");
  return (
    <div
      ref={ref}
      {...list.rootProps}
      onKeyDown={(e) => {
        if (e.target instanceof HTMLElement && e.target.hasAttribute("data-opener"))
          list.selectionKey(e, 0);
      }}
      title={withheld ? t`Explicit content is restricted on this account` : undefined}
      className={cn("min-w-0", withheld && "opacity-50")}
    >
      <div data-row={0} className="flex min-w-0 items-end gap-6">
        {link ? (
          <Link {...link} tabIndex={-1} aria-hidden draggable={false} className="shrink-0">
            {art}
          </Link>
        ) : (
          art
        )}
        <div className="min-w-0 flex-1 pb-1">
          <h3 className="line-clamp-2 text-display text-balance text-ink">
            {link ? (
              <Link {...link} data-opener draggable={false} className={opener}>
                {name}
              </Link>
            ) : (
              <button
                type="button"
                data-opener
                onClick={() => openExternal(item)}
                className={cn(opener, "text-left")}
              >
                {name}
              </button>
            )}
          </h3>
          {item.subtitle && (
            <p className="mt-1.5 truncate text-heading font-medium text-ink">{item.subtitle}</p>
          )}
          {meta && <p className="mt-1 truncate text-meta text-ink-muted">{meta}</p>}
          <div className="mt-5 flex items-center gap-2">
            <PlayButtons
              label={name}
              disabled={!canPlay(item) || withheld}
              onPlay={() => playItem(item)}
            />
            {target && <ItemActionMenu item={target} name={name} variant="header" />}
          </div>
        </div>
      </div>
      {list.menu}
    </div>
  );
}

/** Signed out: where the lead will be, what it needs, and the one action. */
export function SignInLead() {
  return (
    <LeadNotice title={<Trans>Recommendations need Apple Music</Trans>}>
      <p className="max-w-[60ch] text-ui text-ink-muted">
        <Trans>Sign in to see picks, mixes and stations chosen for you.</Trans>
      </p>
      <Button size="sm" className="mt-4" onClick={openAccountDialog}>
        <Trans>Sign in</Trans>
      </Button>
    </LeadNotice>
  );
}

/** Offline with no copy of Home yet: nothing to retry; Sona fetches it when it reconnects. */
export function OfflineLead() {
  return (
    <LeadNotice
      icon={<WifiOff aria-hidden className="size-4 text-ink-muted" strokeWidth={1.75} />}
      title={<Trans>No recommendations while offline</Trans>}
    >
      <p className="max-w-[60ch] text-ui text-ink-muted">
        <Trans>Sona fetches them as soon as it reconnects.</Trans>
      </p>
    </LeadNotice>
  );
}

function LeadNotice({
  icon,
  title,
  children,
}: {
  icon?: ReactNode;
  title: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col items-start gap-1 rounded-lg border border-dashed border-control-line px-6 py-7">
      <h2 className="flex items-center gap-2 text-heading text-ink">
        {icon}
        {title}
      </h2>
      {children}
    </section>
  );
}

/** The lead's shape while Home loads: the cover, the name's lines and the picks' rows. */
export function LeadSkeleton() {
  const { t } = useLingui();
  const ref = useRef<HTMLDivElement>(null);
  const wide = useElementWidth(ref) >= SIDE_BY_SIDE;
  const cover = wide ? 232 : 168;
  return (
    <div ref={ref} aria-busy aria-label={t`Loading`}>
      <div className="mb-4 h-4 w-40 rounded-sm bg-hover" />
      <div
        className={cn("grid gap-x-8 gap-y-6", wide ? "items-end" : "grid-cols-1")}
        style={wide ? { gridTemplateColumns: SIDE_BY_SIDE_COLUMNS } : undefined}
      >
        <div className="flex items-end gap-6">
          <div className="shrink-0 rounded-sm bg-hover" style={{ width: cover, height: cover }} />
          <div className="flex-1 pb-1">
            <div className="h-7 w-3/4 rounded-sm bg-hover" />
            <div className="mt-3 h-4 w-1/2 rounded-sm bg-hover" />
            <div className="mt-7 size-10 rounded-full bg-hover" />
          </div>
        </div>
        <div className="grid grid-cols-[repeat(auto-fill,minmax(248px,1fr))] gap-x-6">
          {Array.from({ length: 8 }, (_, i) => (
            <div key={i} className="flex h-14 items-center gap-3 px-2">
              <div className="size-10 shrink-0 rounded-sm bg-hover" />
              <div className="flex-1">
                <div
                  className="h-3.5 rounded-sm bg-hover"
                  style={{ width: `${60 + (i % 3) * 10}%` }}
                />
                <div className="mt-1.5 h-3 w-1/2 rounded-sm bg-hover" />
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
