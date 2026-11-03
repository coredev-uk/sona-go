import {
  ShelfKind,
  type ArtistDetail,
  type ArtistShelf,
  type CatalogArtist,
} from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { plural } from "@lingui/core/macro";
import { getRouteApi } from "@tanstack/react-router";
import { useCallback, useRef, type ReactNode, type RefObject } from "react";

import { ArtistConcerts } from "@/features/concerts";
import { playSongs, shuffleSongs } from "@/features/player";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import type { CoverItem } from "@/shared/components/CoverGrid";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { DetailError } from "@/shared/components/DetailStates";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { Shelf } from "@/shared/components/Shelf";
import { isLibraryId } from "@/shared/lib/queries";
import { albumTile, artistTile, playlistTile, videoTile } from "@/shared/lib/tiles";
import { useElementWidth } from "@/shared/lib/useCoverColumns";
import { TrackList } from "@/shared/tracks/TrackList";

import {
  artistLine,
  bandViews,
  bioLead,
  CATALOGUE_ROWS,
  catalogueColumns,
  COLUMN_GAP,
  ESSENTIAL_ROWS,
  essentials,
  layoutFor,
  releaseCount,
  shelvesFor,
  TOP_ROWS,
  videoShelf,
} from "./artist";
import { ArtistHead } from "./ArtistHead";
import { ArtistSkeleton } from "./ArtistSkeleton";
import { useCatalogArtist, useLibraryArtist } from "./queries";
import { ReleaseList } from "./ReleaseList";
import { isArtistView, viewTitle } from "./views";

const route = getRouteApi("/artist/$id");

/** The About band's id, for the header's "Read more". */
const ABOUT_ID = "artist-about";

/** A shelf's tiles: the artist's own albums by year, the rest by name. */
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

/** A shelf's "See all" page, when it has more than the page shows. */
function seeAll(artistId: string, view: string, more: boolean) {
  return more
    ? { to: "/artist/$id/view/$view" as const, params: { id: artistId, view } }
    : undefined;
}

/** A cover shelf for one of the artist's views. */
function ViewShelf({ sh, artist }: { sh: ArtistShelf; artist: { id: string; name: string } }) {
  if (!isArtistView(sh.view)) return null;
  return (
    <CoverShelf
      title={viewTitle(sh.view, artist.name)}
      items={shelfTiles(sh)}
      aspect={sh.kind === ShelfKind.ShelfVideos ? "video" : "square"}
      more={seeAll(artist.id, sh.view, sh.more)}
    />
  );
}

/**
 * A band of the page: a 1px line across the column and its heading at the
 * page-title size, then its shelves (docs/design.md, Artist). Focusable so
 * a jump to it moves focus there.
 */
function Band({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section
      id={id}
      tabIndex={-1}
      aria-labelledby={`${id}-title`}
      className="mt-14 scroll-mt-6 border-t border-line pt-6 outline-none"
    >
      <h2 id={`${id}-title`} className="mb-5 text-page text-ink">
        {title}
      </h2>
      <div>{children}</div>
    </section>
  );
}

/** The biography in full at a reading measure, and the facts beside it. */
function About({ page, width }: { page: CatalogArtist; width: number }) {
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
  const { wide } = layoutFor(width);
  return (
    <Band id={ABOUT_ID} title={t`About ${artistName}`}>
      <div
        className="grid gap-x-12 gap-y-6"
        style={{ gridTemplateColumns: wide ? "minmax(0, 32rem) minmax(180px, 1fr)" : "1fr" }}
      >
        {page.bio ? (
          <div className="max-w-[32rem] text-ui whitespace-pre-line text-ink">
            {page.bio.split(/\n{2,}/).map((p, i) => (
              <p key={i} className={i > 0 ? "mt-3" : undefined}>
                {p}
              </p>
            ))}
          </div>
        ) : (
          <div />
        )}
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
    </Band>
  );
}

/**
 * Album shelves as the back catalogue's lists, side by side in `cols`
 * columns, each with its title and "See all". Nothing when every shelf is
 * empty.
 */
function CatalogueLists({
  shelves,
  artist,
  cols,
  by,
}: {
  shelves: ArtistShelf[];
  artist: { id: string; name: string };
  cols: number;
  by?: "year" | "artist";
}) {
  const lists = shelves.filter((sh) => sh.albums.length && isArtistView(sh.view));
  if (!lists.length) return null;
  return (
    <div
      className="mt-10 grid gap-y-10 first:mt-0"
      style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, columnGap: COLUMN_GAP }}
    >
      {lists.map((sh) => {
        const title = isArtistView(sh.view) ? viewTitle(sh.view, artist.name) : "";
        return (
          <Shelf
            key={sh.view}
            className="mt-0"
            title={title}
            more={seeAll(artist.id, sh.view, sh.more || sh.albums.length > CATALOGUE_ROWS)}
          >
            <ReleaseList albums={sh.albums.slice(0, CATALOGUE_ROWS)} label={title} by={by} />
          </Shelf>
        );
      })}
    </div>
  );
}

/**
 * Everything Apple Music has on the artist below the header: the lead (top
 * songs, with the essential albums beside them), then the bands:
 * Discography (albums as covers, singles, live albums and compilations as
 * the back catalogue's lists, music videos), Related (appears on,
 * playlists, similar artists) and About. Library artist pages show it after
 * the library's own shelves.
 */
function CatalogBody({ page, width }: { page: CatalogArtist; width: number }) {
  const { artist, topSongs } = page;
  const onPlay = useCallback(
    (i: number) => playSongs(topSongs, i, { kind: "artist", id: artist.id }),
    [topSongs, artist.id],
  );
  const { t } = useLingui();
  const artistName = artist.name;
  const { wide } = layoutFor(width);

  const essential = essentials(page);
  const essentialAlbums = essential?.albums.slice(0, ESSENTIAL_ROWS) ?? [];
  const top = topSongs.length > 0 && (
    <Shelf
      title={t`Top songs`}
      more={seeAll(artist.id, "top-songs", page.topSongsMore || topSongs.length > TOP_ROWS)}
    >
      <TrackList
        songs={topSongs.slice(0, TOP_ROWS)}
        columns={{ number: "row", artwork: true, album: true }}
        label={t`Top songs by ${artistName}`}
        onPlay={onPlay}
      />
    </Shelf>
  );
  const essentialShelf = essentialAlbums.length > 0 && (
    <Shelf
      title={t`Essential albums`}
      more={seeAll(
        artist.id,
        "featured-albums",
        !!essential?.more || (essential?.albums.length ?? 0) > ESSENTIAL_ROWS,
      )}
      className={top && wide ? "mt-0" : undefined}
    >
      <ReleaseList
        albums={essentialAlbums}
        label={t`Essential albums`}
        columns={wide || !top ? 1 : 2}
      />
    </Shelf>
  );

  const albums = shelvesFor(page, bandViews.albums);
  const catalogue = shelvesFor(page, bandViews.catalogue);
  const videos = videoShelf(page);
  const appears = shelvesFor(page, bandViews.appears);
  const related = shelvesFor(page, bandViews.related);
  // One column width for every catalogue grid on the page, so they line up.
  const cols = catalogueColumns(width, bandViews.catalogue.length);

  return (
    <>
      {(top || essentialShelf) && (
        <div
          className="grid"
          style={{
            gridTemplateColumns: wide && top && essentialShelf ? "minmax(0, 1fr) 300px" : "1fr",
            columnGap: 40,
          }}
        >
          {top}
          {essentialShelf}
        </div>
      )}
      <ArtistConcerts artistId={artist.id} />
      {(albums.length > 0 || catalogue.length > 0 || videos) && (
        <Band id="artist-discography" title={t`Discography`}>
          {albums.map((sh) => (
            <ViewShelf key={sh.view} sh={sh} artist={artist} />
          ))}
          <CatalogueLists shelves={catalogue} artist={artist} cols={cols} />
          {videos && <ViewShelf sh={videos} artist={artist} />}
        </Band>
      )}
      {(appears.length > 0 || related.length > 0) && (
        <Band id="artist-related" title={t`Related`}>
          <CatalogueLists shelves={appears} artist={artist} cols={cols} by="artist" />
          {related.map((sh) => (
            <ViewShelf key={sh.view} sh={sh} artist={artist} />
          ))}
        </Band>
      )}
      <About page={page} width={width} />
    </>
  );
}

/** Scrolls to the About band and moves focus there, so the next Tab is inside it. */
function readMore() {
  const el = document.getElementById(ABOUT_ID);
  if (!el) return;
  el.scrollIntoView({ block: "start" });
  el.focus({ preventScroll: true });
}

/** The page's root, measured: the header and the bands lay out by the content width. */
function usePageWidth(): [RefObject<HTMLDivElement | null>, number] {
  const ref = useRef<HTMLDivElement>(null);
  return [ref, useElementWidth(ref)];
}

function CatalogArtistPage({ page }: { page: CatalogArtist }) {
  const { artist, topSongs } = page;
  const { t } = useLingui();
  const [ref, width] = usePageWidth();
  const artistName = artist.name;
  const lead = bioLead(page.bio);
  const releases = releaseCount(page);
  return (
    <div ref={ref}>
      <ArtistHead
        artist={artist}
        header={page.header}
        line={artistLine(page.genres[0] ?? artist.genre, page.origin, releases)}
        lead={lead}
        onReadMore={lead ? readMore : undefined}
        // An artist with one release has it in the discography; the header doesn't repeat it.
        latest={releases === 1 ? undefined : (page.latestRelease ?? undefined)}
        width={width}
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
      <CatalogBody page={page} width={width} />
    </div>
  );
}

/**
 * A library artist: the albums and songs in your library first, then (for
 * artists Apple Music knows) the same lead and bands as the catalogue page.
 */
function LibraryArtistPage({ detail }: { detail: ArtistDetail }) {
  const { artist, header, albums, songs } = detail;
  const catalog = useCatalogArtist(artist.catalogId, !!artist.catalogId);
  const [ref, width] = usePageWidth();
  const onPlay = useCallback(
    (i: number) => playSongs(songs, i, { kind: "artist", id: artist.id }),
    [songs, artist.id],
  );
  const full = albums.filter((a) => !a.single);
  const singles = albums.filter((a) => a.single);
  const { t } = useLingui();
  const artistName = artist.name;
  const libraryLine = t`${plural(albums.length, { one: "# album", other: "# albums" })}, ${plural(songs.length, { one: "# song", other: "# songs" })} in your library`;
  const page = catalog.data;
  const lead = page ? bioLead(page.bio) : "";
  return (
    <div ref={ref}>
      <ArtistHead
        artist={artist}
        header={header}
        line={artistLine(page?.genres[0] ?? "", "", undefined, libraryLine)}
        lead={lead}
        onReadMore={lead ? readMore : undefined}
        latest={page?.latestRelease ?? undefined}
        width={width}
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
            {page && (
              <>
                <RatingButtons kind="artists" item={page.artist} name={artist.name} />
                <ActionMenu
                  kind="artist"
                  target={page.artist}
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
      {page && (
        <div className="mt-14">
          <CatalogBody page={page} width={width} />
        </div>
      )}
    </div>
  );
}

export function ArtistPage() {
  const { id } = route.useParams();
  const lib = isLibraryId(id, "artist");
  const libQ = useLibraryArtist(id, lib);
  const catQ = useCatalogArtist(id, !lib);
  const q = lib ? libQ : catQ;
  const { t } = useLingui();

  if (q.isPending) return <ArtistSkeleton />;
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
