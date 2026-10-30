import { DiscoverService, type Section } from "@bindings/services";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { Trans, useLingui } from "@lingui/react/macro";
import { RotateCw } from "lucide-react";

import { hasAccount, useAuth } from "@/features/auth";
import { discoverKeys } from "@/features/discover";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { relativeTime } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";
import { EditorialSections } from "@/shared/items/EditorialSections";
import { ItemGrid } from "@/shared/items/ItemGrid";
import type { ItemLinkProps } from "@/shared/items/links";
import { Button } from "@/shared/ui/button";

import { LocalShelves } from "./LocalShelves";

const shelfMore = (s: Section): ItemLinkProps | undefined =>
  s.seeAll ? { to: "/for-you/$id", params: { id: s.id } } : undefined;

/**
 * For you: Apple's personal shelves for this account, read from Go's cache
 * at once (Go renews due shelves after the first paint and pushes them as
 * "discover:home"). Signed out or without any copy yet, what Sona knows
 * locally.
 */
export function HomePage() {
  const { t } = useLingui();
  const auth = useAuth();
  const state = auth.data?.state;
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

  const page = home.data;
  const sections = page?.sections ?? [];
  const fetched = page?.fetchedAt ? relativeTime(page.fetchedAt) : "";
  const subline = page?.fetchedAt
    ? page.stale
      ? t`Couldn't refresh · from ${fetched}`
      : t`Picked for you ${fetched}`
    : undefined;

  return (
    <>
      <PageHeader
        title={t`For you`}
        subline={subline}
        actions={
          signedIn && (
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
      {refresh.isError && (
        <ErrorBlock
          className="mb-8"
          title={t`Couldn't refresh your recommendations`}
          error={refresh.error}
          onRetry={() => refresh.mutate()}
        />
      )}
      {!signedIn ? (
        state === undefined ? null : (
          <>
            <EmptyState
              frame="covers"
              title={t`Recommendations need Apple Music`}
              action={{ label: t`Sign in`, to: "/settings", search: { section: "account" } }}
              className="mb-10"
            >
              <Trans>
                Sign in to see picks, mixes and stations chosen for you. Your library is below.
              </Trans>
            </EmptyState>
            <LocalShelves signedIn={false} />
          </>
        )
      ) : home.isPending ? (
        <HomeSkeleton />
      ) : home.isError ? (
        <>
          <ErrorBlock
            className="mb-10"
            title={t`Couldn't load your recommendations`}
            error={home.error}
            onRetry={() => void home.refetch()}
            retrying={home.isRefetching}
          />
          <LocalShelves signedIn />
        </>
      ) : sections.length ? (
        <EditorialSections sections={sections} moreFor={shelfMore} />
      ) : (
        <LocalShelves signedIn />
      )}
    </>
  );
}

function HomeSkeleton() {
  const { t } = useLingui();
  return (
    <div aria-busy aria-label={t`Loading`} className="flex flex-col gap-10">
      {[208, 160, 160].map((min, s) => (
        <div key={s}>
          <div className="mb-3 h-4 w-40 rounded-sm bg-hover" />
          <div
            className="grid [grid-auto-rows:0] [grid-template-rows:auto] gap-x-6 overflow-hidden"
            style={{ gridTemplateColumns: `repeat(auto-fill, minmax(${min}px, 1fr))` }}
          >
            {Array.from({ length: 10 }, (_, i) => (
              <div key={i}>
                <div className="aspect-square rounded-sm bg-hover" />
                <div className="mt-2 h-3.5 w-3/4 rounded-sm bg-hover" />
                <div className="mt-1.5 h-3 w-1/2 rounded-sm bg-hover" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
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
