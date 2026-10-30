import type { Chart, ChartsPage as Charts, Genre, Song } from "@bindings/services";
import { getRouteApi, Link } from "@tanstack/react-router";
import { ChevronRight } from "lucide-react";

import { playSongs, shuffleSongs } from "@/features/player";
import { PageHeader } from "@/shared/components/PageHeader";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { ItemShelf } from "@/shared/items/ItemShelf";
import { TrackList } from "@/shared/tracks/TrackList";

import { chartTitle } from "./chart-titles";
import { Choice } from "./Choice";
import { useCharts } from "./queries";
import { freshness } from "./freshness";
import { PageError, ShelvesSkeleton } from "./states";

const route = getRouteApi("/charts");

/** How many top songs the overview lists before "See all". */
const TOP_SONGS = 10;

export function GenreChoice({
  genres,
  value,
  onChange,
}: {
  genres: readonly Genre[];
  value: string;
  onChange: (genre: string) => void;
}) {
  if (genres.length < 2) return null;
  return (
    <Choice
      label="Genre"
      value={value}
      options={genres.map((g) => ({ value: g.id, label: g.name }))}
      onChange={onChange}
      width="w-48"
    />
  );
}

/** Charts: Apple Music's most played, overall or by genre, plus city and country charts. */
export function ChartsPage() {
  const { genre = "" } = route.useSearch();
  const navigate = route.useNavigate();
  const q = useCharts(genre);
  const setGenre = (g: string) =>
    void navigate({ search: { genre: g || undefined }, replace: true });
  return (
    <>
      <PageHeader
        title="Charts"
        subline={q.data ? freshness(q.data.fetchedAt, q.data.stale) : undefined}
        actions={q.data && <GenreChoice genres={q.data.genres} value={genre} onChange={setGenre} />}
      />
      {q.isPending ? (
        <ShelvesSkeleton />
      ) : q.isError ? (
        <PageError
          what="the charts"
          error={q.error}
          onRetry={() => void q.refetch()}
          retrying={q.isRefetching}
        />
      ) : (
        <ChartList page={q.data} />
      )}
    </>
  );
}

function ChartList({ page }: { page: Charts }) {
  return page.charts.map((c) => {
    const more = {
      to: "/charts/$chart" as const,
      params: { chart: c.key },
      search: { genre: page.genre || undefined, chartId: c.chartId || undefined },
    };
    if (c.key === "songs") return <TopSongs key={c.key} chart={c} more={more} />;
    return (
      <ItemShelf
        key={`${c.key}:${c.chartId}`}
        title={chartTitle(c)}
        items={c.items}
        more={more}
        min={c.key === "music-videos" ? 240 : 160}
      />
    );
  });
}

/** The top songs, ranked, with Play for the whole chart's first page. */
function TopSongs({
  chart,
  more,
}: {
  chart: Chart;
  more: {
    to: "/charts/$chart";
    params: { chart: string };
    search: { genre?: string; chartId?: string };
  };
}) {
  const songs: Song[] = chart.items.flatMap((i) => (i.song ? [i.song] : []));
  const title = chartTitle(chart);
  return (
    <section aria-label={title} className="mt-2 mb-10">
      <div className="mb-3 flex min-h-8 flex-wrap items-center gap-x-4 gap-y-2">
        <h2 className="text-heading text-ink">{title}</h2>
        <div className="flex items-center gap-2">
          <PlayButtons
            label={title}
            disabled={!songs.some((s) => s.playable)}
            onPlay={() => playSongs(songs, 0, title)}
            onShuffle={() => void shuffleSongs(songs, title)}
          />
        </div>
        <Link
          {...more}
          className="-mr-1 ml-auto inline-flex h-7 items-center gap-0.5 rounded-md px-1.5 text-sub text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink"
        >
          See all
          <ChevronRight aria-hidden className="size-3.5" strokeWidth={1.75} />
        </Link>
      </div>
      <TrackList
        songs={songs.slice(0, TOP_SONGS)}
        columns={{ number: "row", artwork: true, artist: true, album: true }}
        label={title}
        onPlay={(i) => playSongs(songs, i, title)}
      />
    </section>
  );
}
