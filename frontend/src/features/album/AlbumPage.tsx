import type { AlbumDetail, CatalogAlbum } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQueryClient } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { playSongs, shuffleSongs } from "@/features/player";
import { RemindButton } from "@/features/reminders";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { ArtistLinks } from "@/shared/components/ArtistLinks";
import { Badges } from "@/shared/components/Badges";
import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { DetailError, DetailSkeleton } from "@/shared/components/DetailStates";
import { LibraryButton } from "@/shared/components/LibraryButton";
import { Notes } from "@/shared/components/Notes";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { Shelf } from "@/shared/components/Shelf";
import { catalogueLine, longDate, yearOf } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { isLibraryId } from "@/shared/lib/queries";
import { isUpcoming } from "@/shared/lib/release";
import { albumTile, playlistTile, videoTile } from "@/shared/lib/tiles";
import { useNow } from "@/shared/lib/useNow";
import { TrackList } from "@/shared/tracks/TrackList";

import { useCatalogAlbum, useLibraryAlbum } from "./queries";
import { ReleaseStatus } from "./ReleaseStatus";

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
  const tracks = data?.tracks;
  const albumId = data?.album.id ?? id;
  const onPlay = useCallback(
    (i: number) => tracks && playSongs(tracks, i, { kind: "album", id: albumId }),
    [tracks, albumId],
  );
  const popular = useMemo(() => new Set(page?.popularIds), [page]);
  const { t } = useLingui();
  const auth = useAuth();
  const qc = useQueryClient();
  // An album not out yet counts down to the minute; at its release the
  // page reloads it so its songs become playable.
  const now = useNow(60_000);
  const loaded = data?.album;
  const released = !!loaded?.prerelease && !isUpcoming(loaded, now);
  useEffect(() => {
    if (!released) return;
    void qc.invalidateQueries({ queryKey: lib ? keys.libraryAlbum(id) : keys.catalogAlbum(id) });
  }, [released, qc, lib, id]);

  if (q.isPending) return <DetailSkeleton />;
  if (q.error) {
    return (
      <DetailError
        noun="album"
        error={q.error}
        retry={() => void q.refetch()}
        retrying={q.isRefetching}
        back={{ label: t`Go to Albums`, to: "/library/albums" }}
      />
    );
  }

  const { album, header, durationMs } = q.data;
  const recordLabel = q.data.recordLabel || page?.recordLabel || "";
  const copyright = q.data.copyright || page?.copyright || "";
  const notes = q.data.notes || page?.notes || "";
  const list = q.data.tracks;
  const variousArtists = list.some((t) => t.artistName !== album.artistName);
  const kind = album.single ? t`Single` : t`Album`;
  const releaseDate = longDate(album.releaseDate);
  const albumName = album.name;
  const upcoming = isUpcoming(album, now);
  // The header carries Apple's one-line note; the full notes follow the tracks.
  const lede = page?.notesShort || undefined;
  const meta = [
    catalogueLine(kind, yearOf(album.releaseDate), list.length, durationMs),
    album.genre,
  ]
    .filter(Boolean)
    .join(" · ");
  const songsOut = list.filter((s) => s.playable).length;
  return (
    <>
      <CollectionHeader
        title={album.name}
        art={album.art.large}
        // A library album plays the motion cover of its catalogue album.
        header={page && !header.motion ? { ...header, motion: page.header.motion } : header}
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
        description={lede}
        status={
          upcoming && <ReleaseStatus album={album} now={now} out={songsOut} total={list.length} />
        }
        actions={
          upcoming ? (
            // Not out yet: no Play (its songs already out play from their
            // rows); a reminder, and Apple's pre-add while signed in.
            <>
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
                disabled={!list.some((t) => t.playable)}
                onPlay={() => playSongs(list, 0, { kind: "album", id: album.id })}
                onShuffle={() => void shuffleSongs(list, { kind: "album", id: album.id })}
              />
              <LibraryButton kind="albums" item={album} name={album.name} />
              <RatingButtons kind="albums" item={album} name={album.name} />
              <ActionMenu kind="album" target={album} name={album.name} variant="header" />
            </>
          )
        }
      />
      <TrackList
        songs={list}
        columns={{ number: "track", artist: variousArtists }}
        label={t`Songs on ${albumName}`}
        onPlay={onPlay}
        discs
        highlight={song}
        popular={popular}
      />
      <footer className="mt-6 px-2 text-meta text-ink-muted">
        {album.releaseDate && (
          <p>
            {upcoming ? <Trans>Out {releaseDate}</Trans> : <Trans>Released {releaseDate}</Trans>}
          </p>
        )}
        {copyright && <p className="mt-0.5">{copyright}</p>}
        {/* The label, unless the copyright line already names it. */}
        {recordLabel && !copyright.includes(recordLabel) && <p className="mt-0.5">{recordLabel}</p>}
      </footer>
      {notes && notes !== lede && (
        <Shelf title={t`Notes`} className="mt-10">
          <Notes text={notes} />
        </Shelf>
      )}
      {page && <CatalogSections page={page} />}
    </>
  );
}
