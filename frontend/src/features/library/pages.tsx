import { PlaybackService, type Artist } from "@bindings/services";
import { plural } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";
import { getRouteApi, Link } from "@tanstack/react-router";
import { memo, useEffect, useRef } from "react";

import { replaceQueue } from "@/features/player";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { CoverGrid, type CoverItem } from "@/shared/components/CoverGrid";
import { Artwork } from "@/shared/components/Artwork";
import { PageHeader } from "@/shared/components/PageHeader";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { TrackList, type SongSort } from "@/shared/tracks/TrackList";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { run } from "@/shared/lib/run";
import { useContentVirtualizer } from "@/shared/lib/useContentVirtualizer";

import { skeletonRows, skeletonTiles, useLibraryGate } from "./gate";
import { useLibraryList } from "./queries";

function SortSelect<S extends string>({
  value,
  options,
  onChange,
}: {
  value: S;
  options: { value: S; label: string }[];
  onChange: (v: S) => void;
}) {
  const { t } = useLingui();
  return (
    <Select value={value} onValueChange={(v) => onChange(v as S)}>
      <SelectTrigger aria-label={t`Sort by`} className="h-8 w-40 text-ui">
        <SelectValue />
      </SelectTrigger>
      <SelectContent align="end">
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Sorts that read best newest-first start descending. */
const startsDesc = (sort: string) =>
  sort === "dateAdded" || sort === "releaseDate" || sort === "lastModified";

const songsRoute = getRouteApi("/library/songs");

export function LibrarySongsPage() {
  const { t } = useLingui();
  const search = songsRoute.useSearch();
  const navigate = songsRoute.useNavigate();
  const sort: SongSort = search.sort ?? "title";
  const desc = search.desc ?? false;
  const list = useLibraryList("songs", sort, desc);
  const gate = useLibraryGate({
    total: list.total,
    pending: list.isPending,
    error: list.error,
    retry: () => void list.refetch(),
    retrying: list.isRefetching,
    kind: "songs",
    empty: t`Songs you add in Apple Music on any device appear here after the next sync.`,
    frame: "box",
  });

  const setSort = (next: SongSort, nextDesc = startsDesc(next)) =>
    void navigate({ search: { sort: next, desc: nextDesc }, replace: true });
  const play = (start: number) =>
    void replaceQueue(() =>
      run(t`Couldn't start playback`, PlaybackService.PlayLibrary(sort, desc, start)),
    );
  const shuffle = async () => {
    const loaded = list.rows.flatMap((s, i) => (s?.playable ? [i] : []));
    const start = loaded.length ? loaded[Math.floor(Math.random() * loaded.length)] : 0;
    if (await run(t`Couldn't turn on shuffle`, PlaybackService.SetShuffle(true))) play(start);
  };

  return (
    <>
      <PageHeader
        title={t`Songs`}
        subline={list.total ? plural(list.total, { one: "# song", other: "# songs" }) : undefined}
        actions={
          <>
            <SortSelect
              value={sort}
              onChange={(v) => setSort(v)}
              options={[
                { value: "title", label: t`Title` },
                { value: "artist", label: t`Artist` },
                { value: "album", label: t`Album` },
                { value: "dateAdded", label: t`Recently added` },
              ]}
            />
            <PlayButtons
              playLabel={t`Play your songs`}
              disabled={!list.total}
              onPlay={() => play(0)}
              onShuffle={() => void shuffle()}
            />
          </>
        }
      />
      {gate ?? (
        <TrackList
          songs={list.isPending ? skeletonRows : list.rows}
          columns={{ number: "row", artwork: true, artist: true, album: true }}
          label={t`Songs in your library`}
          onPlay={play}
          onRange={list.onRange}
          sort={{
            key: sort,
            desc,
            onSort: (key) => (key === sort ? setSort(key, !desc) : setSort(key)),
          }}
        />
      )}
    </>
  );
}

const albumsRoute = getRouteApi("/library/albums");

export function LibraryAlbumsPage() {
  const { t } = useLingui();
  const search = albumsRoute.useSearch();
  const navigate = albumsRoute.useNavigate();
  const sort = search.sort ?? "dateAdded";
  const desc = search.desc ?? startsDesc(sort);
  const list = useLibraryList("albums", sort, desc);
  const gate = useLibraryGate({
    total: list.total,
    pending: list.isPending,
    error: list.error,
    retry: () => void list.refetch(),
    retrying: list.isRefetching,
    kind: "albums",
    empty: t`Albums you add in Apple Music appear here after the next sync, newest first.`,
    frame: "covers",
  });
  const items: (CoverItem | undefined)[] = list.isPending
    ? skeletonTiles
    : list.rows.map(
        (a) =>
          a && {
            key: a.id,
            title: a.name,
            subtitle: a.artistName,
            art: a.art.medium,
            explicit: a.explicit,
            link: { to: "/album/$id", params: { id: a.id } },
            target: { kind: "album", target: a },
          },
      );
  return (
    <>
      <PageHeader
        title={t`Albums`}
        subline={list.total ? plural(list.total, { one: "# album", other: "# albums" }) : undefined}
        actions={
          <SortSelect
            value={sort}
            onChange={(v) =>
              void navigate({ search: { sort: v, desc: startsDesc(v) }, replace: true })
            }
            options={[
              { value: "dateAdded", label: t`Recently added` },
              { value: "title", label: t`Title` },
              { value: "artist", label: t`Artist` },
              { value: "releaseDate", label: t`Release date` },
            ]}
          />
        }
      />
      {gate ?? <CoverGrid items={items} label={t`Albums in your library`} onRange={list.onRange} />}
    </>
  );
}

const ARTIST_ROW = 52;

/** One artist row; memoised so rows already on screen skip re-rendering as the range moves. */
const ArtistRow = memo(function ArtistRow({ artist: a }: { artist: Artist | undefined }) {
  if (!a) {
    return (
      <div aria-hidden className="flex h-[52px] items-center gap-3 px-2">
        <div className="size-9 rounded-full bg-hover" />
        <div className="h-3.5 w-40 rounded-sm bg-hover" />
      </div>
    );
  }
  return (
    <div className="group/row flex h-[52px] items-center gap-2 rounded-md pr-2 transition-colors duration-100 hover:bg-hover">
      <Link
        to="/artist/$id"
        params={{ id: a.id }}
        className="flex h-full min-w-0 flex-1 items-center gap-3 rounded-md px-2 text-ui text-ink focus-visible:bg-hover focus-visible:outline-offset-[-2px]"
      >
        <Artwork src={a.art.small} size={36} alt="" shape="round" />
        <span className="truncate">{a.name}</span>
      </Link>
      <ActionMenu kind="artist" target={a} name={a.name} />
    </div>
  );
});

function ArtistRows({
  rows,
  onRange,
}: {
  rows: readonly (Artist | undefined)[];
  onRange: (start: number, end: number) => void;
}) {
  const { t } = useLingui();
  const ref = useRef<HTMLDivElement>(null);
  const { items, totalSize, margin } = useContentVirtualizer(ref, rows.length, ARTIST_ROW, 10);
  const first = items[0]?.index ?? 0;
  const last = items[items.length - 1]?.index ?? -1;
  useEffect(() => {
    if (last >= 0) onRange(first, last);
  }, [onRange, first, last]);
  return (
    <div ref={ref} role="list" aria-label={t`Artists in your library`}>
      <div className="relative" style={{ height: totalSize }}>
        {items.map((it) => (
          <div
            key={it.key}
            role="listitem"
            className="absolute inset-x-0 top-0"
            style={{ transform: `translateY(${it.start - margin}px)` }}
          >
            <ArtistRow artist={rows[it.index]} />
          </div>
        ))}
      </div>
    </div>
  );
}

export function LibraryArtistsPage() {
  const { t } = useLingui();
  const list = useLibraryList("artists", "title", false);
  const gate = useLibraryGate({
    total: list.total,
    pending: list.isPending,
    error: list.error,
    retry: () => void list.refetch(),
    retrying: list.isRefetching,
    kind: "artists",
    empty: t`Every artist with a song or album in your library is listed here, A to Z.`,
    frame: "box",
  });
  return (
    <>
      <PageHeader
        title={t`Artists`}
        subline={
          list.total ? plural(list.total, { one: "# artist", other: "# artists" }) : undefined
        }
      />
      {gate ?? (
        <ArtistRows rows={list.isPending ? skeletonRows : list.rows} onRange={list.onRange} />
      )}
    </>
  );
}

const playlistsRoute = getRouteApi("/library/playlists");

export function LibraryPlaylistsPage() {
  const { t } = useLingui();
  const search = playlistsRoute.useSearch();
  const navigate = playlistsRoute.useNavigate();
  const sort = search.sort ?? "dateAdded";
  const desc = search.desc ?? startsDesc(sort);
  const list = useLibraryList("playlists", sort, desc);
  const gate = useLibraryGate({
    total: list.total,
    pending: list.isPending,
    error: list.error,
    retry: () => void list.refetch(),
    retrying: list.isRefetching,
    kind: "playlists",
    empty: t`Playlists you make or add in Apple Music appear here after the next sync.`,
    frame: "covers",
  });
  const items: (CoverItem | undefined)[] = list.isPending
    ? skeletonTiles
    : list.rows.map(
        (p) =>
          p && {
            key: p.id,
            title: p.name,
            subtitle:
              p.curatorName ||
              (p.trackCount
                ? plural(p.trackCount, { one: "# song", other: "# songs" })
                : undefined),
            art: p.art.medium,
            link: { to: "/playlist/$id", params: { id: p.id } },
            target: { kind: "playlist", target: p },
          },
      );
  return (
    <>
      <PageHeader
        title={t`Playlists`}
        subline={
          list.total ? plural(list.total, { one: "# playlist", other: "# playlists" }) : undefined
        }
        actions={
          <SortSelect
            value={sort}
            onChange={(v) =>
              void navigate({ search: { sort: v, desc: startsDesc(v) }, replace: true })
            }
            options={[
              { value: "dateAdded", label: t`Recently added` },
              { value: "lastModified", label: t`Recently updated` },
              { value: "title", label: t`Title` },
            ]}
          />
        }
      />
      {gate ?? (
        <CoverGrid items={items} label={t`Playlists in your library`} onRange={list.onRange} />
      )}
    </>
  );
}
