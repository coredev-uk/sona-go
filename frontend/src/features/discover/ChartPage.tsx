import { DiscoverService, type Item, type Song } from "@bindings/services";
import { getRouteApi } from "@tanstack/react-router";
import { useCallback, useMemo, type ReactNode } from "react";

import { playSongs, shuffleSongs } from "@/features/player";
import { EmptyState } from "@/shared/components/EmptyState";
import { PageHeader } from "@/shared/components/PageHeader";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { ItemGrid } from "@/shared/items/ItemGrid";
import { TrackList } from "@/shared/tracks/TrackList";

import { chartTitle } from "./chart-titles";
import { GenreChoice } from "./ChartsPage";
import { discoverKeys, useCharts, useMorePages } from "./queries";
import { PageError, ShelvesSkeleton } from "./states";

const route = getRouteApi("/charts/$chart");

/** One chart in full, in pages of 50 as it scrolls. */
export function ChartRoutePage() {
  const { chart: key } = route.useParams();
  const { genre = "", chartId = "" } = route.useSearch();
  const navigate = route.useNavigate();
  const q = useCharts(genre);
  if (q.isPending) return <ShelvesSkeleton />;
  if (q.isError) {
    return (
      <PageError
        what="this chart"
        error={q.error}
        onRetry={() => void q.refetch()}
        retrying={q.isRefetching}
      />
    );
  }
  const chart = q.data.charts.find((c) => c.key === key && (!chartId || c.chartId === chartId));
  const genreChoice = (
    <GenreChoice
      genres={q.data.genres}
      value={genre}
      onChange={(g) =>
        void navigate({
          search: { genre: g || undefined, chartId: chartId || undefined },
          replace: true,
        })
      }
    />
  );
  if (!chart) {
    return (
      <>
        <PageHeader title="Charts" actions={genreChoice} />
        <EmptyState title="No chart here" action={{ label: "All charts", to: "/charts" }}>
          Apple Music has no such chart for this genre.
        </EmptyState>
      </>
    );
  }
  const genreName = q.data.genres.find((g) => g.id === genre)?.name;
  return (
    <ChartContent
      key={`${key}|${chart.chartId}|${genre}`}
      title={chartTitle(chart)}
      subline={genre && genreName ? genreName : undefined}
      chartKey={key}
      chartId={chart.chartId}
      genre={genre}
      first={chart.items}
      next={chart.next}
      actions={genreChoice}
    />
  );
}

function ChartContent({
  title,
  subline,
  chartKey,
  chartId,
  genre,
  first,
  next,
  actions,
}: {
  title: string;
  subline?: string;
  chartKey: string;
  chartId: string;
  genre: string;
  first: Item[];
  next: number;
  actions: ReactNode;
}) {
  const more = useMorePages(discoverKeys.chart(chartKey, chartId, genre), next, (offset) =>
    DiscoverService.ChartItems(chartKey, chartId, genre, offset),
  );
  const items = useMemo(
    () => [...first, ...(more.data?.pages.flatMap((p) => p.items) ?? [])],
    [first, more.data],
  );
  const hasMore = next > 0 && (!more.data || more.hasNextPage);
  const { fetchNextPage, isFetching } = more;
  const onRange = useCallback(
    (_: number, end: number) => {
      if (hasMore && !isFetching && end >= items.length - 12) void fetchNextPage();
    },
    [hasMore, isFetching, items.length, fetchNextPage],
  );

  if (chartKey === "songs") {
    const songs: Song[] = items.flatMap((i) => (i.song ? [i.song] : []));
    return (
      <>
        <PageHeader
          title={title}
          subline={subline}
          actions={
            <>
              <PlayButtons
                label={title}
                disabled={!songs.some((s) => s.playable)}
                onPlay={() => playSongs(songs, 0, title)}
                onShuffle={() => void shuffleSongs(songs, title)}
              />
              {actions}
            </>
          }
        />
        <TrackList
          songs={hasMore ? [...songs, undefined] : songs}
          columns={{ number: "row", artwork: true, artist: true, album: true }}
          label={title}
          onPlay={(i) => playSongs(songs, i, title)}
          onRange={onRange}
        />
      </>
    );
  }
  return (
    <>
      <PageHeader title={title} subline={subline} actions={actions} />
      <ItemGrid
        items={hasMore ? [...items, ...Array<undefined>(6)] : items}
        label={title}
        onRange={onRange}
        min={chartKey === "music-videos" ? 240 : 160}
      />
    </>
  );
}
