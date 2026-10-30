import type { CatalogSong, CreditGroup } from "@bindings/services";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useState } from "react";

import { playSongs } from "@/features/player";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { ArtistLinks } from "@/shared/components/ArtistLinks";
import { Badges } from "@/shared/components/Badges";
import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { DetailError, DetailSkeleton } from "@/shared/components/DetailStates";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { LibraryButton } from "@/shared/components/LibraryButton";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { Shelf, ShelfToggle } from "@/shared/components/Shelf";
import { formatTime, longDate, yearOf } from "@/shared/lib/format";
import { albumTile, videoTile } from "@/shared/lib/tiles";

import { creditTitle } from "./credits";
import { useFullLyrics, useSongPage } from "./queries";

const route = getRouteApi("/song/$id");

/** The excerpt Apple sends, and the whole lyrics on request. */
function LyricsBlock({ page }: { page: CatalogSong }) {
  const [full, setFull] = useState(false);
  const lyrics = useFullLyrics(page.song.catalogId || page.song.id, full);
  const lines =
    full && lyrics.data ? lyrics.data.lines.map((l) => l.text) : page.lyricsExcerpt.split("\n");
  return (
    <Shelf
      title="Lyrics"
      actions={
        <ShelfToggle open={full} onToggle={() => setFull(!full)}>
          {full ? "Show less" : "Show all lyrics"}
        </ShelfToggle>
      }
    >
      <div className="max-w-[60ch] text-heading leading-7 font-medium text-ink">
        {lines.map((l, i) => (
          // A blank line in the lyrics is a verse break.
          <p key={i} className={l ? undefined : "h-4"}>
            {l}
          </p>
        ))}
        {!full && <p className="text-ink-faint">…</p>}
      </div>
      {full && lyrics.isPending && (
        <p className="mt-2 text-sub text-ink-muted">Loading the lyrics…</p>
      )}
      {full && lyrics.error && (
        <ErrorBlock
          className="mt-3"
          title="Couldn't load the lyrics"
          error={lyrics.error}
          onRetry={() => void lyrics.refetch()}
          retrying={lyrics.isRefetching}
        />
      )}
      {full && !!lyrics.data?.songwriters.length && (
        <p className="mt-4 text-sub text-ink-muted">
          Written by {lyrics.data.songwriters.join(", ")}
        </p>
      )}
    </Shelf>
  );
}

function Credits({ groups }: { groups: CreditGroup[] }) {
  return (
    <Shelf title="Credits">
      <div className="grid gap-8">
        {groups.map((g) => (
          <section key={g.kind} aria-label={creditTitle(g)}>
            <h3 className="mb-2 text-sub font-semibold text-ink-muted">{creditTitle(g)}</h3>
            <dl className="grid gap-x-8 gap-y-2.5 min-[720px]:grid-cols-2">
              {g.credits.map((c) => (
                <div key={c.name} className="min-w-0">
                  <dt className="truncate text-ui text-ink">{c.name}</dt>
                  <dd className="text-sub text-ink-muted">{c.roles.join(", ")}</dd>
                </div>
              ))}
            </dl>
          </section>
        ))}
      </div>
    </Shelf>
  );
}

/**
 * A song's own page: who made it (credits), its lyrics, its album and
 * music videos, and more by its artist.
 */
export function SongPage() {
  const { id } = route.useParams();
  const q = useSongPage(id);

  if (q.isPending) return <DetailSkeleton />;
  if (q.error) {
    return (
      <DetailError
        noun="song"
        error={q.error}
        retry={() => void q.refetch()}
        retrying={q.isRefetching}
        back={{ label: "Go to Songs", to: "/library/songs" }}
      />
    );
  }
  const page = q.data;
  const { song, album } = page;
  const meta = ["Song", yearOf(song.releaseDate), formatTime(song.durationMs), song.genre]
    .filter(Boolean)
    .join(" · ");
  const hasLyrics = !!page.lyricsExcerpt;
  const hasCredits = page.credits.length > 0;

  return (
    <>
      <CollectionHeader
        title={song.name}
        art={song.art.large}
        header={page.header}
        byline={
          <>
            <ArtistLinks artists={page.artists} fallback={song.artistName} />
            {album && (
              <span className="text-ink-muted">
                {" · "}
                <Link
                  to="/album/$id"
                  params={{ id: album.id }}
                  search={{ song: song.id }}
                  className="hover:text-ink hover:underline hover:underline-offset-2"
                >
                  {album.name}
                </Link>
              </span>
            )}
          </>
        }
        meta={meta}
        badges={<Badges explicit={song.explicit} traits={song.traits} />}
        actions={
          <>
            <PlayButtons
              label={song.name}
              disabled={!song.playable}
              onPlay={() => playSongs([song], 0, song.id)}
            />
            <LibraryButton kind="songs" item={song} name={song.name} />
            <RatingButtons kind="songs" item={song} name={song.name} />
            <ActionMenu kind="song" target={song} name={song.name} variant="header" />
          </>
        }
      />
      {(hasLyrics || hasCredits) && (
        <div className="grid gap-x-16 gap-y-10 min-[1100px]:grid-cols-2 [&>section]:mt-0">
          {hasLyrics && <LyricsBlock page={page} />}
          {hasCredits && <Credits groups={page.credits} />}
        </div>
      )}
      {!hasLyrics && !hasCredits && (
        <p className="text-sub text-ink-muted">
          Apple Music has no credits or lyrics for this song.
        </p>
      )}
      <CoverShelf
        title="Music videos"
        aspect="video"
        items={page.videos.map((v) => videoTile(v, "year"))}
      />
      <CoverShelf
        title={`More by ${song.artistName}`}
        items={page.moreByArtist.map((a) => albumTile(a, "year"))}
        more={
          song.artistId
            ? { to: "/artist/$id/view/$view", params: { id: song.artistId, view: "full-albums" } }
            : undefined
        }
      />
      <footer className="mt-8 px-0.5 text-meta text-ink-muted">
        {song.releaseDate && <p>Released {longDate(song.releaseDate)}</p>}
        {page.composer && <p className="mt-0.5">Composed by {page.composer}</p>}
        {page.isrc && <p className="mt-0.5">ISRC {page.isrc}</p>}
      </footer>
    </>
  );
}
