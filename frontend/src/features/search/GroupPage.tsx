import type { Item } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useRef } from "react";

import { playSongs } from "@/features/player";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { ItemGrid } from "@/shared/items/ItemGrid";
import { TrackList } from "@/shared/tracks/TrackList";

import { groupEmpty, groupLoadError, groupTitle } from "./groups";
import { useGroupPages, useRecentActions } from "./queries";

const route = getRouteApi("/search/$group");
const CONTENT_ID = "content";
const ROOT_MARGIN = "100% 0px";

/** Loads the next page when the end of the list comes within a screen. */
function useLoadMore(hasMore: boolean, loading: boolean, load: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el || !hasMore) return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting) && !loading) load();
      },
      { root: document.getElementById(CONTENT_ID), rootMargin: ROOT_MARGIN },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [hasMore, loading, load]);
  return ref;
}

/** "See all" for one result group: every page Apple has, loaded as it scrolls. */
export function GroupPage() {
  const { t } = useLingui();
  const { group } = route.useParams();
  const { q = "" } = route.useSearch();
  const pages = useGroupPages(q, group);
  const { rememberTerm, rememberItem } = useRecentActions();
  const items = useMemo(() => pages.data?.pages.flatMap((p) => p.items) ?? [], [pages.data]);
  const songs = useMemo(
    () => (group === "song" ? items.flatMap((i) => (i.song ? [i.song] : [])) : []),
    [group, items],
  );
  const onOpen = useCallback(
    (item: Item) => {
      rememberTerm(q, "catalog");
      rememberItem(item);
    },
    [q, rememberTerm, rememberItem],
  );
  const onPlay = useCallback(
    (i: number) => {
      onOpen(items[i]);
      playSongs(songs, i, "search");
    },
    [items, songs, onOpen],
  );
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = pages;
  const loadMore = useCallback(() => void fetchNextPage(), [fetchNextPage]);
  const sentinel = useLoadMore(hasNextPage, isFetchingNextPage, loadMore);

  const title = groupTitle(group);
  const header = (
    <PageHeader
      title={title}
      subline={
        <>
          <Trans>
            Apple Music results for{" "}
            <Link
              to="/search"
              search={{ q }}
              className="text-ink underline decoration-ink/40 underline-offset-2 hover:decoration-ink"
            >
              “{q}”
            </Link>
          </Trans>
        </>
      }
    />
  );

  if (pages.error && !items.length) {
    return (
      <>
        {header}
        <ErrorBlock
          title={groupLoadError(group)}
          error={pages.error}
          onRetry={() => void pages.refetch()}
          retrying={pages.isRefetching}
        />
      </>
    );
  }
  if (pages.isSuccess && !items.length) {
    return (
      <>
        {header}
        <EmptyState title={groupEmpty(group, q)}>
          <Trans>Apple Music has nothing more of this kind for the search.</Trans>
        </EmptyState>
      </>
    );
  }
  // Placeholder rows for the page that is loading.
  const pending = pages.isPending || isFetchingNextPage ? 6 : 0;
  return (
    <>
      {header}
      {group === "song" ? (
        <TrackList
          songs={[...songs, ...Array<undefined>(pending)]}
          columns={{ number: "row", artwork: true, artist: true, album: true }}
          label={t`${title} matching ${q}`}
          onPlay={onPlay}
        />
      ) : (
        <ItemGrid
          items={[...items, ...Array<undefined>(pending)]}
          label={t`${title} matching ${q}`}
          onOpen={onOpen}
        />
      )}
      {pages.error && items.length > 0 && (
        <ErrorBlock
          className="mt-6"
          title={t`Couldn't load more`}
          error={pages.error}
          onRetry={() => void fetchNextPage()}
        />
      )}
      <div ref={sentinel} aria-hidden className="h-px" />
    </>
  );
}
