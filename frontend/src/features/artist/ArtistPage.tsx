import {
  ShelfKind,
  type Album,
  type ArtistDetail,
  type ArtistShelf,
  type CatalogArtist,
} from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { plural } from "@lingui/core/macro";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useCallback } from "react";

import { playSongs, shuffleSongs } from "@/features/player";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { FluidArtwork } from "@/shared/components/Artwork";
import { CollectionHeader } from "@/shared/components/CollectionHeader";
import type { CoverItem } from "@/shared/components/CoverGrid";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { DetailError, DetailSkeleton } from "@/shared/components/DetailStates";
import { Notes } from "@/shared/components/Notes";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { Shelf } from "@/shared/components/Shelf";
import { longDate } from "@/shared/lib/format";
import { isLibraryId } from "@/shared/lib/queries";
import { albumTile, artistTile, playlistTile, videoTile } from "@/shared/lib/tiles";
import { TrackList } from "@/shared/tracks/TrackList";

import { useCatalogArtist, useLibraryArtist } from "./queries";
import { isArtistView, viewTitle } from "./views";

const route = getRouteApi("/artist/$id");

/** Top songs shown on the page; the rest are behind "See all". */
const TOP_ROWS = 6;

/** A shelf's tiles: albums by year (it is the artist's own list), the rest by name. */
function shelfTiles(sh: ArtistShelf): CoverItem[] {
  switch (sh.kind) {
    case ShelfKind.ShelfAlbums:
      return sh.albums.map((a) =>
        albumTile(a, sh.view === "appears-on-albums" ? "artist" : "year"),
      );
    case ShelfKind.ShelfPlaylists:
      return sh.playlists.map(playlistTile);
    case ShelfKind.ShelfVideos:
      return sh.videos.map((v) => videoTile(v, "year"));
    case ShelfKind.ShelfArtists:
      return sh.artists.map(artistTile);
    default:
      return [];
  }
}

function ReleaseCard({ album }: { album: Album }) {
  const { t } = useLingui();
  const kind = album.single ? t`Single` : t`Album`;
  const trackCount = album.trackCount;
  return (
    <Link
      to="/album/$id"
      params={{ id: album.id }}
      className="group/tile flex w-full min-w-0 flex-col rounded-sm outline-offset-4"
    >
      <FluidArtwork src={album.art.medium} size={240} alt="" />
      <span className="mt-2 text-meta text-ink-muted num">
        {longDate(album.releaseDate, "short")}
      </span>
      <span className="mt-0.5 line-clamp-2 text-ui font-medium text-ink group-hover/tile:underline group-hover/tile:underline-offset-2">
        {album.name}
      </span>
      <span className="text-sub text-ink-muted num">
        {[kind, trackCount ? plural(trackCount, { one: "# song", other: "# songs" }) : ""]
          .filter(Boolean)
          .join(" · ")}
      </span>
    </Link>
  );
}

function About({ page }: { page: CatalogArtist }) {
  const { t } = useLingui();
  const facts: [string, string][] = [];
  if (page.origin) facts.push([t`From`, page.origin]);
  if (page.bornOrFormed) facts.push([t`Born or formed`, page.bornOrFormed]);
  const genreCount = page.genres.length;
  if (genreCount) {
    facts.push([plural(genreCount, { one: "Genre", other: "Genres" }), page.genres.join(", ")]);
  }
  const artistName = page.artist.name;
  if (!page.bio && !facts.length) return null;
  return (
    <Shelf title={t`About ${artistName}`}>
      <div className="grid gap-x-12 gap-y-6 min-[960px]:grid-cols-[minmax(0,68ch)_minmax(180px,1fr)]">
        {page.bio ? <Notes text={page.bio} lines={6} /> : <div />}
        {facts.length > 0 && (
          <dl className="grid content-start gap-3">
            {facts.map(([term, value]) => (
              <div key={term}>
                <dt className="text-meta text-ink-muted">{term}</dt>
                <dd className="text-ui text-ink">{value}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>
    </Shelf>
  );
}

/**
 * Everything Apple Music has on the artist below the header: latest
 * release and top songs, the shelves (each with "See all"), and About.
 * Library artist pages show it after the library's own sections.
 */
function CatalogBody({ page }: { page: CatalogArtist }) {
  const { artist, topSongs, latestRelease } = page;
  const onPlay = useCallback(
    (i: number) => playSongs(topSongs, i, { kind: "artist", id: artist.id }),
    [topSongs, artist.id],
  );
  const { t } = useLingui();
  const artistName = artist.name;
  const top = topSongs.length > 0 && (
    <Shelf
      title={t`Top songs`}
      more={
        page.topSongsMore || topSongs.length > TOP_ROWS
          ? { to: "/artist/$id/view/$view", params: { id: artist.id, view: "top-songs" } }
          : undefined
      }
    >
      <TrackList
        songs={topSongs.slice(0, TOP_ROWS)}
        columns={{ number: "row", artwork: true, album: true }}
        label={t`Top songs by ${artistName}`}
        onPlay={onPlay}
      />
    </Shelf>
  );
  return (
    <>
      {latestRelease ? (
        <div className="mt-10 grid gap-10 first:mt-0 min-[960px]:grid-cols-[200px_minmax(0,1fr)]">
          <Shelf title={t`Latest release`}>
            <ReleaseCard album={latestRelease} />
          </Shelf>
          <div className="min-w-0 [&>section]:mt-0">{top}</div>
        </div>
      ) : (
        top
      )}
      {page.shelves.map((sh) => {
        if (!isArtistView(sh.view)) return null;
        const video = sh.kind === ShelfKind.ShelfVideos;
        return (
          <CoverShelf
            key={sh.view}
            title={viewTitle(sh.view, artist.name)}
            items={shelfTiles(sh)}
            aspect={video ? "video" : "square"}
            more={
              sh.more
                ? { to: "/artist/$id/view/$view", params: { id: artist.id, view: sh.view } }
                : undefined
            }
          />
        );
      })}
      <About page={page} />
    </>
  );
}

function CatalogArtistPage({ page }: { page: CatalogArtist }) {
  const { artist, topSongs } = page;
  const { t } = useLingui();
  const artistName = artist.name;
  const meta = [page.genres[0] ?? artist.genre, page.origin].filter(Boolean).join(" · ");
  return (
    <>
      <CollectionHeader
        title={artist.name}
        art={artist.art.large}
        shape="round"
        header={page.header}
        meta={meta}
        actions={
          <>
            {topSongs.length > 0 && (
              <PlayButtons
                playLabel={t`Play ${artistName}'s top songs`}
                disabled={!topSongs.some((s) => s.playable)}
                onPlay={() => playSongs(topSongs, 0, { kind: "artist", id: artist.id })}
                onShuffle={() => void shuffleSongs(topSongs, { kind: "artist", id: artist.id })}
              />
            )}
            <RatingButtons kind="artists" item={artist} name={artist.name} />
            <ActionMenu kind="artist" target={artist} name={artist.name} variant="header" />
          </>
        }
      />
      <CatalogBody page={page} />
    </>
  );
}

/**
 * A library artist: the albums and songs in your library first, then (for
 * artists Apple Music knows) the same sections as the catalogue page.
 */
function LibraryArtistPage({ detail }: { detail: ArtistDetail }) {
  const { artist, header, albums, songs } = detail;
  const catalog = useCatalogArtist(artist.catalogId, !!artist.catalogId);
  const onPlay = useCallback(
    (i: number) => playSongs(songs, i, { kind: "artist", id: artist.id }),
    [songs, artist.id],
  );
  const full = albums.filter((a) => !a.single);
  const singles = albums.filter((a) => a.single);
  const { t } = useLingui();
  const artistName = artist.name;
  const libraryLine = t`${plural(albums.length, { one: "# album", other: "# albums" })}, ${plural(songs.length, { one: "# song", other: "# songs" })} in your library`;
  return (
    <>
      <CollectionHeader
        title={artist.name}
        art={artist.art.large}
        shape="round"
        header={header}
        meta={[catalog.data?.genres[0], libraryLine].filter(Boolean).join(" · ")}
        actions={
          <>
            {songs.length > 0 && (
              <PlayButtons
                label={artist.name}
                disabled={!songs.some((s) => s.playable)}
                onPlay={() => playSongs(songs, 0, { kind: "artist", id: artist.id })}
                onShuffle={() => void shuffleSongs(songs, { kind: "artist", id: artist.id })}
              />
            )}
            {catalog.data && (
              <>
                <RatingButtons kind="artists" item={catalog.data.artist} name={artist.name} />
                <ActionMenu
                  kind="artist"
                  target={catalog.data.artist}
                  name={artist.name}
                  variant="header"
                />
              </>
            )}
          </>
        }
      />
      <CoverShelf title={t`Albums in your library`} items={full.map((a) => albumTile(a, "year"))} />
      <CoverShelf
        title={t`Singles and EPs in your library`}
        items={singles.map((a) => albumTile(a, "year"))}
      />
      {songs.length > 0 && (
        <Shelf title={t`Songs in your library`}>
          <TrackList
            songs={songs}
            columns={{ number: "row", artwork: true, album: true }}
            label={t`Songs by ${artistName} in your library`}
            onPlay={onPlay}
          />
        </Shelf>
      )}
      {catalog.data && <CatalogBody page={catalog.data} />}
    </>
  );
}

export function ArtistPage() {
  const { id } = route.useParams();
  const lib = isLibraryId(id, "artist");
  const libQ = useLibraryArtist(id, lib);
  const catQ = useCatalogArtist(id, !lib);
  const q = lib ? libQ : catQ;
  const { t } = useLingui();

  if (q.isPending) return <DetailSkeleton round />;
  if (q.error) {
    return (
      <DetailError
        noun="artist"
        error={q.error}
        retry={() => void q.refetch()}
        retrying={q.isRefetching}
        back={{ label: t`Go to Artists`, to: "/library/artists" }}
      />
    );
  }
  if (libQ.data) return <LibraryArtistPage detail={libQ.data} />;
  return catQ.data ? <CatalogArtistPage page={catQ.data} /> : null;
}
