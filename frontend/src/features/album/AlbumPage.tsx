import type { Album, AlbumDetail, CatalogAlbum, Song } from "@bindings/services";
import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQueryClient } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useRef } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { playSongs, shuffleSongs } from "@/features/player";
import { RemindButton } from "@/features/reminders";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { ArtistLinks } from "@/shared/components/ArtistLinks";
import { Badges } from "@/shared/components/Badges";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { DetailError } from "@/shared/components/DetailStates";
import { LibraryButton } from "@/shared/components/LibraryButton";
import { Notes } from "@/shared/components/Notes";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { Shelf } from "@/shared/components/Shelf";
import { useContentPolicy } from "@/shared/lib/content-policy";
import { catalogueLine, longDate, yearOf } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { isLibraryId } from "@/shared/lib/queries";
import { isUpcoming, releaseBadge } from "@/shared/lib/release";
import { albumTile, playlistTile, videoTile } from "@/shared/lib/tiles";
import { cn } from "@/shared/lib/utils";
import { useNow } from "@/shared/lib/useNow";
import { TrackList, VIRTUAL_FROM } from "@/shared/tracks/TrackList";

import {
  discsOf,
  isCompilation,
  libraryShare,
  withoutPlaceholderCredits,
  type Disc,
} from "./album";
import { AlbumHead } from "./AlbumHead";
import { AlbumSkeleton } from "./AlbumSkeleton";
import { AlbumCover, Credits, DiscIndex, Sleeve } from "./AlbumSleeve";
import { albumLayout, type AlbumLayout } from "./layout";
import { useCatalogAlbum, useLibraryAlbum } from "./queries";
import { ReleaseStatus } from "./ReleaseStatus";
import { useFitsInPane, useWidth } from "./measure";

const route = getRouteApi("/album/$id");

const LINK_CLS = "hover:underline hover:underline-offset-2";

/** Apple's sections for the album, in its order. */
function CatalogSections({ page }: { page: CatalogAlbum }) {
  const { album } = page;
  const { t } = useLingui();
  const artistName = album.artistName;
  return (
    <>
      <CoverShelf
        title={t`Other versions`}
        items={page.otherVersions.map((a) => albumTile(a, "year"))}
      />
      <CoverShelf
        title={t`Music videos`}
        aspect="video"
        items={page.videos.map((v) => videoTile(v, "year"))}
      />
      <CoverShelf
        title={t`More by ${artistName}`}
        items={page.moreByArtist.map((a) => albumTile(a, "year"))}
        more={
          album.artistId
            ? { to: "/artist/$id/view/$view", params: { id: album.artistId, view: "full-albums" } }
            : undefined
        }
      />
      <CoverShelf title={t`Featured in playlists`} items={page.appearsOn.map(playlistTile)} />
      <CoverShelf title={t`You might also like`} items={page.similar.map((a) => albumTile(a))} />
    </>
  );
}

function isCatalogPage(d: AlbumDetail | CatalogAlbum): d is CatalogAlbum {
  return "moreByArtist" in d;
}

/**
 * A library album that holds only some of the record's songs says so where
 * the missing songs would be, with the way to the whole album.
 */
function LibraryShare({
  share,
  catalogId,
}: {
  share: { have: number; total: number };
  catalogId: string;
}) {
  const { have, total } = share;
  const line = plural(total, {
    one: `${have} of # song in your library`,
    other: `${have} of # songs in your library`,
  });
  return (
    <p className="text-sub text-ink">
      {line}
      {" · "}
      <Link
        to="/album/$id"
        params={{ id: catalogId }}
        className="text-ink underline underline-offset-2"
      >
        <Trans>Show complete album</Trans>
      </Link>
    </p>
  );
}

/**
 * The line under a song's title on the album: a credit other than the
 * album artist's (when the list has no Artist column), then why the song
 * doesn't play, so a greyed row is never left unexplained.
 */
function useSubline(album: Album, compilation: boolean, upcoming: boolean, now: number) {
  const { t } = useLingui();
  const restricted = useContentPolicy((s) => s.restricted);
  const releaseDate = longDate(album.releaseDate);
  const out = (upcoming && releaseBadge(album, now)) || t`Out ${releaseDate}`;
  const unavailable = t`Unavailable`;
  const withheld = t`Explicit content is restricted on this account`;
  return useCallback(
    (song: Song) => {
      const credit =
        !compilation && song.artistName && song.artistName !== album.artistName
          ? song.artistName
          : "";
      const reason = !song.playable
        ? upcoming
          ? out
          : unavailable
        : restricted && song.explicit
          ? withheld
          : "";
      return [credit, reason].filter(Boolean).join(" · ");
    },
    [album.artistName, compilation, upcoming, out, unavailable, restricted, withheld],
  );
}

export function AlbumPage() {
  const { id } = route.useParams();
  const { song } = route.useSearch();
  const lib = isLibraryId(id, "album");
  const libQ = useLibraryAlbum(id, lib);
  const catQ = useCatalogAlbum(id, !lib);
  const q = lib ? libQ : catQ;
  const data = q.data;
  // A library album Apple Music also has shows the same sections as its catalogue page.
  const catalogId = lib ? (libQ.data?.album.catalogId ?? "") : "";
  const linked = useCatalogAlbum(catalogId, !!catalogId);
  const page = data && isCatalogPage(data) ? data : linked.data;
  const { t } = useLingui();
  const qc = useQueryClient();
  const rootRef = useRef<HTMLDivElement>(null);
  const layout = albumLayout(useWidth(rootRef));
  // An album not out yet counts down to the minute; at its release the
  // page reloads it so its songs become playable.
  const now = useNow(60_000);
  const loaded = data?.album;
  const released = !!loaded?.prerelease && !isUpcoming(loaded, now);
  useEffect(() => {
    if (!released) return;
    void qc.invalidateQueries({ queryKey: lib ? keys.libraryAlbum(id) : keys.catalogAlbum(id) });
  }, [released, qc, lib, id]);

  let body;
  if (q.isPending) body = <AlbumSkeleton layout={layout} />;
  else if (q.error) {
    body = (
      <DetailError
        noun="album"
        error={q.error}
        retry={() => void q.refetch()}
        retrying={q.isRefetching}
        back={{ label: t`Go to Albums`, to: "/library/albums" }}
      />
    );
  } else {
    body = <AlbumBody detail={q.data} page={page} layout={layout} now={now} highlight={song} />;
  }
  return <div ref={rootRef}>{body}</div>;
}

function AlbumBody({
  detail,
  page,
  layout,
  now,
  highlight,
}: {
  detail: AlbumDetail | CatalogAlbum;
  /** Apple's catalogue page for the album (a library album's linked one). */
  page: CatalogAlbum | undefined;
  layout: AlbumLayout;
  now: number;
  highlight?: string;
}) {
  const { t } = useLingui();
  const auth = useAuth();
  const listRef = useRef<HTMLDivElement>(null);
  const sleeveRef = useRef<HTMLDivElement>(null);
  // The sleeve stays beside the songs while they scroll only when the pane shows all of it.
  const sticky = useFitsInPane(sleeveRef, 28, layout.wide);
  const { album, header, durationMs } = detail;
  const upcoming = isUpcoming(album, now);
  const list = useMemo(
    () => withoutPlaceholderCredits(detail.tracks, upcoming),
    [detail.tracks, upcoming],
  );
  const albumId = album.id;
  const onPlay = useCallback(
    (i: number) => playSongs(list, i, { kind: "album", id: albumId }),
    [list, albumId],
  );
  const popular = useMemo(() => new Set(page?.popularIds), [page]);
  const compilation = isCompilation(list, album.artistName);
  const subline = useSubline(album, compilation, upcoming, now);
  // A list long enough to be virtualised has no disc headings to jump to.
  const discs: Disc[] = useMemo(() => (list.length > VIRTUAL_FROM ? [] : discsOf(list)), [list]);

  const recordLabel = detail.recordLabel || page?.recordLabel || "";
  const copyright = detail.copyright || page?.copyright || "";
  const notes = detail.notes || page?.notes || "";
  // The header carries Apple's one-line note; the full notes follow the songs.
  const lede = page?.notesShort || undefined;
  const share = isCatalogPage(detail) ? undefined : libraryShare(list.length, page?.tracks.length);
  // The catalogue line describes the record: a partial library album
  // counts the whole album; an album not out yet leaves out a running time
  // its unpublished songs can't add to.
  const songs = share ? share.total : list.length;
  const running = share
    ? page?.durationMs
    : upcoming && list.some((s) => !s.durationMs)
      ? undefined
      : durationMs;
  const kind = album.single ? t`Single` : t`Album`;
  const meta = [catalogueLine(kind, yearOf(album.releaseDate), songs, running), album.genre]
    .filter(Boolean)
    .join(" · ");
  const songsOut = list.filter((s) => s.playable).length;
  const releaseDate = longDate(album.releaseDate);
  const credits = [
    // An album not out yet says its day in the countdown.
    !upcoming && album.releaseDate ? t`Released ${releaseDate}` : "",
    // The label, unless the copyright line already names it.
    recordLabel && !copyright.includes(recordLabel) ? recordLabel : "",
    copyright,
  ].filter(Boolean);
  // A library album plays the motion cover of its catalogue album.
  const motion = header.motion || page?.header.motion || "";
  const albumName = album.name;
  const playOut = plural(songsOut, {
    one: "Play the # song out now",
    other: "Play the # songs out now",
  });

  const head = (
    <AlbumHead
      title={album.name}
      byline={
        page ? (
          <ArtistLinks artists={page.artists} fallback={album.artistName} />
        ) : album.artistId ? (
          <Link to="/artist/$id" params={{ id: album.artistId }} className={LINK_CLS}>
            {album.artistName}
          </Link>
        ) : (
          album.artistName
        )
      }
      meta={meta}
      badges={<Badges explicit={album.explicit} traits={album.traits} />}
      lede={lede}
      status={
        upcoming ? (
          <ReleaseStatus album={album} now={now} out={songsOut} total={list.length} />
        ) : (
          share && <LibraryShare share={share} catalogId={album.catalogId} />
        )
      }
      actions={
        upcoming ? (
          // Not out yet: Play takes the songs already out; a reminder, and
          // Apple's pre-add while signed in.
          <>
            {songsOut > 0 && <PlayButtons playLabel={playOut} onPlay={() => onPlay(0)} />}
            <RemindButton album={album} />
            {hasAccount(auth.data?.state) && (
              <LibraryButton kind="albums" item={album} name={album.name} addLabel={t`Pre-add`} />
            )}
            <RatingButtons kind="albums" item={album} name={album.name} />
            <ActionMenu kind="album" target={album} name={album.name} variant="header" />
          </>
        ) : (
          <>
            <PlayButtons
              label={album.name}
              disabled={songsOut === 0}
              onPlay={() => onPlay(0)}
              onShuffle={() => void shuffleSongs(list, { kind: "album", id: album.id })}
            />
            <LibraryButton kind="albums" item={album} name={album.name} />
            <RatingButtons kind="albums" item={album} name={album.name} />
            <ActionMenu kind="album" target={album} name={album.name} variant="header" />
          </>
        )
      }
    />
  );

  const tracks = (
    <div ref={listRef}>
      <TrackList
        songs={list}
        columns={{ number: "track", artist: compilation }}
        label={t`Songs on ${albumName}`}
        onPlay={onPlay}
        discs
        highlight={highlight}
        popular={popular}
        subline={subline}
      />
    </div>
  );
  const notesShelf = notes && notes !== lede && (
    <Shelf title={t`Notes`} className="mt-10">
      <Notes text={notes} />
    </Shelf>
  );
  const cover = <AlbumCover art={album.art.large} motion={motion} size={layout.cover} />;

  return (
    <>
      {layout.wide ? (
        <div
          className="grid items-start gap-x-10"
          style={{
            gridTemplateColumns: `${layout.cover + 32}px minmax(0,1fr)`,
            // The last row takes up whatever height the sleeve needs past a short listing.
            gridTemplateRows: "auto auto auto minmax(0,1fr)",
          }}
        >
          {/* Reading order: the head, the sleeve (its disc index leads into the songs), the songs, the notes. */}
          <div className="col-start-2 row-start-1 min-w-0 pt-4">{head}</div>
          <Sleeve
            ref={sleeveRef}
            header={header}
            className={cn("col-start-1 row-span-4 row-start-1", sticky && "sticky top-7")}
          >
            <div className="p-4">
              {cover}
              {discs.length > 0 && (
                <div className="mt-4">
                  <DiscIndex discs={discs} list={listRef} />
                </div>
              )}
              {credits.length > 0 && (
                <div className="mt-4">
                  <Credits lines={credits} />
                </div>
              )}
            </div>
          </Sleeve>
          <div className="col-start-2 row-start-2 mt-7 min-w-0">{tracks}</div>
          {notesShelf && <div className="col-start-2 row-start-3 mt-10 min-w-0">{notesShelf}</div>}
        </div>
      ) : (
        <>
          <Sleeve header={header}>
            <div className="flex items-start gap-5 p-5">
              {cover}
              {head}
            </div>
          </Sleeve>
          <div className="mt-6">{tracks}</div>
          {/* The tracks carry their disc headings; at this width the sleeve's index would sit below them. */}
          {credits.length > 0 && (
            <div className="mt-6">
              <Credits lines={credits} />
            </div>
          )}
          {notesShelf}
        </>
      )}
      {page && (
        <div className="mt-12">
          <CatalogSections page={page} />
        </div>
      )}
    </>
  );
}
