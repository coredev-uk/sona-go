import { PlaybackService } from "@bindings/services";
import { plural } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";
import { getRouteApi } from "@tanstack/react-router";

import { replaceQueue } from "@/features/player";
import { CoverGrid, type CoverItem } from "@/shared/components/CoverGrid";
import { PageHeader } from "@/shared/components/PageHeader";
import { EmptyState } from "@/shared/components/EmptyState";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { Segmented } from "@/shared/components/Segmented";
import { TrackList, type SongSort } from "@/shared/tracks/TrackList";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { run } from "@/shared/lib/run";

import { skeletonRows, skeletonTiles, useLibraryGate } from "./gate";
import { useCounts, useLibraryList } from "./queries";
import type { AlbumType } from "./search-params";

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

/** The album type control's choices: a type, or all (no type in the URL). */
type AlbumFilter = AlbumType | "all";

export function LibraryAlbumsPage() {
  const { t } = useLingui();
  const search = albumsRoute.useSearch();
  const navigate = albumsRoute.useNavigate();
  const sort = search.sort ?? "dateAdded";
  const desc = search.desc ?? startsDesc(sort);
  const type = search.type;
  const list = useLibraryList("albums", sort, desc, type);
  const counts = useCounts();
  const gate = useLibraryGate({
    // A type with no albums must not read as an empty library.
    total: type ? Math.max(list.total, counts.data?.albums ?? 0) : list.total,
    pending: list.isPending,
    error: list.error,
    retry: () => void list.refetch(),
    retrying: list.isRefetching,
    kind: "albums",
    empty: t`Albums you add in Apple Music appear here after the next sync, newest first.`,
    frame: "covers",
  });
  const setType = (next: AlbumFilter) =>
    void navigate({
      search: (prev) => ({ ...prev, type: next === "all" ? undefined : next }),
      replace: true,
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
  // A type with nothing in it still says so, so the header does not jump when the filter changes.
  const subline =
    list.isPending || (!list.total && !type)
      ? undefined
      : type === "singles"
        ? plural(list.total, { one: "# single or EP", other: "# singles & EPs" })
        : plural(list.total, { one: "# album", other: "# albums" });
  const noneTitle =
    type === "singles"
      ? t`No singles or EPs in your library`
      : t`No albums besides singles and EPs`;
  const filteredNone = !!type && !list.isPending && !list.total;
  // What a chosen type shows, read out when it changes (a filter's result is not otherwise announced).
  const announcement = !type || list.isPending ? "" : filteredNone ? noneTitle : subline;
  return (
    <>
      <PageHeader
        title={t`Albums`}
        subline={subline}
        actions={
          <>
            <Segmented
              value={type ?? "all"}
              label={t`Album type`}
              onChange={setType}
              options={[
                { value: "all", label: t`All` },
                { value: "albums", label: t`Albums` },
                { value: "singles", label: t`Singles & EPs` },
              ]}
            />
            <SortSelect
              value={sort}
              onChange={(v) =>
                void navigate({
                  search: (prev) => ({ ...prev, sort: v, desc: startsDesc(v) }),
                  replace: true,
                })
              }
              options={[
                { value: "dateAdded", label: t`Recently added` },
                { value: "title", label: t`Title` },
                { value: "artist", label: t`Artist` },
                { value: "releaseDate", label: t`Release date` },
              ]}
            />
          </>
        }
      />
      <div role="status" className="sr-only">
        <p key={announcement}>{announcement}</p>
      </div>
      {gate ??
        (filteredNone ? (
          <EmptyState
            frame="covers"
            title={noneTitle}
            action={{ label: t`Show all albums`, onClick: () => setType("all") }}
          >
            {type === "singles"
              ? t`Singles and EPs you add in Apple Music appear here after the next sync.`
              : t`Everything in your library is a single or an EP.`}
          </EmptyState>
        ) : (
          <CoverGrid
            items={items}
            label={
              type === "singles" ? t`Singles and EPs in your library` : t`Albums in your library`
            }
            onRange={list.onRange}
          />
        ))}
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
