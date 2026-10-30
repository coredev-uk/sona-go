import { ShelfKind, type ArtistViewPage as ViewPage } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useCallback, useMemo } from "react";

import { playSongs } from "@/features/player";
import { CoverGrid } from "@/shared/components/CoverGrid";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { albumTile, artistTile, playlistTile, videoTile } from "@/shared/lib/tiles";
import type { CoverItem } from "@/shared/components/CoverGrid";
import { TrackList } from "@/shared/tracks/TrackList";

import { useArtistView, useCatalogArtist } from "./queries";
import { isArtistView, viewTitle } from "./views";

const route = getRouteApi("/artist/$id/view/$view");

/** One fetched page as tiles, in the list its kind fills. */
function pageTiles(p: ViewPage): CoverItem[] {
  switch (p.kind) {
    case ShelfKind.ShelfAlbums:
      return p.albums.map((a) => albumTile(a, p.view === "appears-on-albums" ? "artist" : "year"));
    case ShelfKind.ShelfPlaylists:
      return p.playlists.map(playlistTile);
    case ShelfKind.ShelfVideos:
      return p.videos.map((v) => videoTile(v, "year"));
    case ShelfKind.ShelfArtists:
      return p.artists.map(artistTile);
    default:
      return [];
  }
}

/**
 * An artist shelf's full list ("See all"): songs as a track list, the rest
 * as a cover grid, fetched page by page as it scrolls.
 */
export function ArtistViewPage() {
  const { id, view } = route.useParams();
  const known = isArtistView(view);
  const artist = useCatalogArtist(id, known);
  const q = useArtistView(id, view, known);
  const pages = q.data?.pages;
  const songs = useMemo(() => pages?.flatMap((p) => p.songs) ?? [], [pages]);
  const tiles = useMemo(() => pages?.flatMap(pageTiles) ?? [], [pages]);
  const kind = pages?.[0]?.kind;
  const video = view === "music-videos" || view === "top-music-videos";
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = q;
  const total = kind === ShelfKind.ShelfSongs ? songs.length : tiles.length;
  // Fetch the next page when the last loaded rows come into view.
  const onRange = useCallback(
    (_: number, end: number) => {
      if (end >= total - 10 && hasNextPage && !isFetchingNextPage) void fetchNextPage();
    },
    [total, hasNextPage, isFetchingNextPage, fetchNextPage],
  );
  const onPlay = useCallback(
    (i: number) => playSongs(songs, i, { kind: "artist", id }),
    [songs, id],
  );

  const { t } = useLingui();
  const name = artist.data?.artist.name ?? "";
  const title = known ? viewTitle(view, name || t`this artist`) : t`Not found`;
  const header = (
    <PageHeader
      title={title}
      subline={
        <Link
          to="/artist/$id"
          params={{ id }}
          className="hover:text-ink hover:underline hover:underline-offset-2"
        >
          {name || t`Artist`}
        </Link>
      }
    />
  );

  if (!known) {
    return (
      <>
        {header}
        <EmptyState
          title={t`There's no such list`}
          action={{ label: t`Go to Artists`, to: "/library/artists" }}
        >
          <Trans>This artist page doesn't have a list called “{view}”.</Trans>
        </EmptyState>
      </>
    );
  }
  if (q.error) {
    return (
      <>
        {header}
        <ErrorBlock
          title={t`Couldn't load this list`}
          error={q.error}
          onRetry={() => void q.refetch()}
          retrying={q.isRefetching}
        />
      </>
    );
  }
  const loading = q.isPending ? Array.from<undefined>({ length: 12 }) : [];
  // Rows and tiles still to come are drawn as placeholders while they load.
  const pending = isFetchingNextPage ? Array.from<undefined>({ length: 6 }) : [];
  return (
    <>
      {header}
      {view === "top-songs" ? (
        <TrackList
          songs={[...songs, ...loading, ...pending]}
          columns={{ number: "row", artwork: true, album: true }}
          label={title}
          onPlay={onPlay}
          onRange={onRange}
        />
      ) : (
        <CoverGrid
          items={[...tiles, ...loading, ...pending]}
          label={title}
          onRange={onRange}
          aspect={video ? "video" : "square"}
          min={video ? 240 : 160}
        />
      )}
      {!q.isPending && total === 0 && (
        <EmptyState title={t`Nothing here yet`} className="mt-2">
          {name ? (
            <Trans>Apple Music has nothing in this list for {name} in your storefront.</Trans>
          ) : (
            <Trans>Apple Music has nothing in this list for this artist in your storefront.</Trans>
          )}
        </EmptyState>
      )}
    </>
  );
}
