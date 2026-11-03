import { DiscoverService, type Section } from "@bindings/services";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { Trans, useLingui } from "@lingui/react/macro";
import { ChevronDown, RotateCw } from "lucide-react";
import { useState } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { ConcertsShelf } from "@/features/concerts";
import { discoverKeys } from "@/features/discover";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { Shelf, ShelfToggle } from "@/shared/components/Shelf";
import { relativeTime } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";
import { EditorialSections } from "@/shared/items/EditorialSections";
import { ItemGrid } from "@/shared/items/ItemGrid";
import { ItemShelf } from "@/shared/items/ItemShelf";
import type { ItemLinkProps } from "@/shared/items/links";
import { Button } from "@/shared/ui/button";

import { HeroCard } from "./HeroCard";
import { HomeLead, LeadBand, LeadSkeleton, OfflineLead, SignInLead } from "./HomeLead";
import { ItemRows } from "./ItemRows";
import { LocalShelves } from "./LocalShelves";
import { isStations, planHome, RECENTLY_PLAYED, withoutSharedCaption } from "./plan";
import { ShelfIndex } from "./ShelfIndex";

const shelfMore = (s: Section): ItemLinkProps | undefined =>
  s.seeAll ? { to: "/for-you/$id", params: { id: s.id } } : undefined;

/**
 * For you: Apple's personal shelves for this account, read from Go's cache
 * at once (Go renews due shelves after the first paint and pushes them as
 * "discover:home"), set as a lead (Top Picks), the shelves Apple ranks
 * first, and an index of the rest (plan.ts). Signed out, offline without a
 * copy yet, or when Home is empty: what Sona knows locally.
 */
export function HomePage() {
  const { t } = useLingui();
  const auth = useAuth();
  const state = auth.data?.state;
  const offline = !!auth.data?.offline;
  const signedIn = hasAccount(state);
  const qc = useQueryClient();
  const home = useQuery({
    queryKey: discoverKeys.home,
    queryFn: () => DiscoverService.Home(),
    enabled: signedIn,
    staleTime: 60_000,
  });
  const refresh = useMutation({
    mutationFn: () => DiscoverService.RefreshHome(),
    onSuccess: (page) => qc.setQueryData(discoverKeys.home, page),
  });

  const page = signedIn ? home.data : undefined;
  const plan = planHome(page?.sections ?? []);
  const fetched = page?.fetchedAt ? relativeTime(page.fetchedAt) : "";
  const subline = page?.fetchedAt
    ? page.stale
      ? t`Couldn't refresh · from ${fetched}`
      : t`Picked for you ${fetched}`
    : undefined;

  const header = (
    <PageHeader
      title={t`For you`}
      subline={subline}
      actions={
        // Offline a refresh can only fail; Go renews Home when it reconnects.
        signedIn &&
        !offline && (
          <Button
            variant="ghost"
            size="sm"
            disabled={refresh.isPending}
            onClick={() => refresh.mutate()}
          >
            <RotateCw
              strokeWidth={1.75}
              className={cn(refresh.isPending && "animate-spin motion-reduce:animate-none")}
            />
            {refresh.isPending ? <Trans>Refreshing…</Trans> : <Trans>Refresh</Trans>}
          </Button>
        )
      }
    />
  );
  const refreshError = refresh.isError && (
    <ErrorBlock
      className="mb-8"
      title={t`Couldn't refresh your recommendations`}
      error={refresh.error}
      onRetry={() => refresh.mutate()}
    />
  );

  if (state === "restoring" || (signedIn && home.isPending)) {
    // Restoring: Go is still reading the sign-in from the keyring, so not signed out.
    return (
      <>
        {header}
        <LeadSkeleton />
      </>
    );
  }
  if (!signedIn) {
    if (state === undefined) return header;
    return (
      <>
        {header}
        <SignInLead />
        <LocalShelves signedIn={false} />
      </>
    );
  }
  if (home.isError) {
    return (
      <>
        {header}
        {offline ? (
          <OfflineLead />
        ) : (
          <ErrorBlock
            title={t`Couldn't load your recommendations`}
            error={home.error}
            onRetry={() => void home.refetch()}
            retrying={home.isRefetching}
          />
        )}
        <LocalShelves signedIn />
      </>
    );
  }
  if (!plan.lead && !plan.open.length && !plan.index.length) {
    return (
      <>
        {header}
        {refreshError}
        <LocalShelves signedIn />
      </>
    );
  }
  return (
    <>
      <LeadBand lead={plan.lead?.items[0]}>
        {header}
        {refreshError}
        {plan.lead && <HomeLead section={plan.lead} />}
      </LeadBand>
      {plan.open.map((s) => (
        <HomeSection key={s.id} section={s} />
      ))}
      {plan.index.length > 0 && <ShelfIndex sections={plan.index} moreFor={shelfMore} />}
    </>
  );
}

/** One shelf shown in full, set by what it holds. */
function HomeSection({ section }: { section: Section }) {
  const items = withoutSharedCaption(section.items);
  switch (section.kind) {
    case "hero":
      return section.items.length === 1 ? (
        <HeroCard item={section.items[0]} />
      ) : (
        <EditorialSections sections={[section]} moreFor={shelfMore} />
      );
    case "feature":
      return <ItemShelf title={section.title} items={items} more={shelfMore(section)} min={176} />;
    case "concerts":
      return <ConcertsShelf />;
    case "shelf":
      return section.source === RECENTLY_PLAYED || isStations(section) ? (
        <RowShelf section={section} />
      ) : (
        <ItemShelf title={section.title} items={items} more={shelfMore(section)} />
      );
    default:
      return <EditorialSections sections={[section]} moreFor={shelfMore} />;
  }
}

/** Rows shown before "Show all" on a row shelf. */
const ROW_SHELF_ROWS = 2;

/**
 * Recently Played and station shelves as compact rows: what you go back to
 * and what plays at once need a play control and a name more than a large
 * cover.
 */
function RowShelf({ section }: { section: Section }) {
  const [cols, setCols] = useState(0);
  const [open, setOpen] = useState(false);
  const more = shelfMore(section);
  const hidden = cols > 0 && section.items.length > cols * ROW_SHELF_ROWS;
  const count = section.items.length;
  return (
    <Shelf
      title={section.title}
      more={more}
      actions={
        !more &&
        (hidden || open) && (
          <ShelfToggle open={open} onToggle={() => setOpen((o) => !o)}>
            {open ? <Trans>Show fewer</Trans> : <Trans>Show all {count}</Trans>}
            <ChevronDown
              aria-hidden
              className={cn("size-3.5 transition-transform duration-100", open && "rotate-180")}
              strokeWidth={1.75}
            />
          </ShelfToggle>
        )
      }
    >
      <ItemRows
        items={withoutSharedCaption(section.items)}
        label={section.title}
        rows={ROW_SHELF_ROWS}
        open={open}
        onColumns={setCols}
      />
    </Shelf>
  );
}

const shelfRoute = getRouteApi("/for-you/$id");

/** A Home shelf in full (up to 100 items). */
export function HomeShelfPage() {
  const { t } = useLingui();
  const { id } = shelfRoute.useParams();
  const q = useQuery({
    queryKey: discoverKeys.homeShelf(id),
    queryFn: () => DiscoverService.HomeShelf(id),
  });
  if (q.isError) {
    return (
      <ErrorBlock
        title={t`Couldn't load this shelf`}
        error={q.error}
        onRetry={() => void q.refetch()}
        retrying={q.isRefetching}
      />
    );
  }
  return (
    <>
      <PageHeader title={q.data?.title ?? ""} />
      <ItemGrid
        items={q.data?.items ?? Array<undefined>(12).fill(undefined)}
        label={q.data?.title ?? t`Loading`}
      />
    </>
  );
}
