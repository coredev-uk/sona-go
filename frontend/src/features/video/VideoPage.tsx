import { CatalogService } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { ExternalLink, Play } from "lucide-react";

import { playSongs, playVideos } from "@/features/player";
import { Badges } from "@/shared/components/Badges";
import { ArtistLinks } from "@/shared/components/ArtistLinks";
import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { DetailError, DetailSkeleton } from "@/shared/components/DetailStates";
import { Notes } from "@/shared/components/Notes";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { Shelf } from "@/shared/components/Shelf";
import { useCapabilities } from "@/shared/lib/capabilities";
import { formatTime, longDate, yearOf } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { run } from "@/shared/lib/run";
import { videoTile } from "@/shared/lib/tiles";
import { Button } from "@/shared/ui/button";

const route = getRouteApi("/music-video/$id");

/** Format badges: technical names, the same in every language. */
const FORMAT_4K = "4K";
const FORMAT_HDR = "HDR";

/**
 * A music video's page. Where videos play (Capabilities.playback.video)
 * the video plays here, in standard definition, in the player's video
 * view; elsewhere the page says so. Either way it opens in Apple Music on
 * the web, and the song it belongs to plays here.
 */
export function VideoPage() {
  const { id } = route.useParams();
  const q = useQuery({
    queryKey: keys.catalogVideo(id),
    queryFn: () => CatalogService.MusicVideo(id),
  });
  // Undefined until Go answers: neither the play control nor the note yet.
  const videoPlays = useCapabilities()?.playback.video;
  const { t } = useLingui();

  if (q.isPending) return <DetailSkeleton />;
  if (q.error) {
    return (
      <DetailError
        noun="music video"
        error={q.error}
        retry={() => void q.refetch()}
        retrying={q.isRefetching}
        back={{ label: t`Go to Search`, to: "/search" }}
      />
    );
  }
  const { video, song } = q.data;
  const songLine = (s: NonNullable<typeof song>) => {
    const link = (
      <Link
        to="/song/$id"
        params={{ id: s.catalogId || s.id }}
        className="text-ink underline decoration-ink/40 underline-offset-2 hover:decoration-ink"
      >
        {s.name}
      </Link>
    );
    const albumName = s.albumName;
    return albumName ? (
      <Trans>
        The song is {link} from {albumName}.
      </Trans>
    ) : (
      <Trans>The song is {link}.</Trans>
    );
  };
  const artistName = video.artistName;
  const releaseDate = longDate(video.releaseDate);
  const meta = [
    t`Music video`,
    yearOf(video.releaseDate),
    formatTime(video.durationMs),
    video.genre,
  ]
    .filter(Boolean)
    .join(" · ");
  const formats = [video.has4K && FORMAT_4K, video.hasHDR && FORMAT_HDR].filter(
    (f): f is string => !!f,
  );

  return (
    <>
      <CollectionHeader
        title={video.name}
        art={video.art.large}
        aspect="video"
        header={q.data.header}
        byline={<ArtistLinks artists={q.data.artists} fallback={video.artistName} />}
        meta={meta}
        badges={<Badges explicit={video.explicit} extra={formats} />}
        actions={
          <>
            {videoPlays && (
              <PlayButtons label={video.name} onPlay={() => void playVideos([video], 0)} />
            )}
            {video.url && (
              <Button
                size="sm"
                variant={videoPlays ? "outline" : "default"}
                onClick={() =>
                  void run(t`Couldn't open Apple Music`, CatalogService.OpenInAppleMusic(video.url))
                }
              >
                <ExternalLink strokeWidth={1.75} />
                <Trans>Watch in Apple Music</Trans>
              </Button>
            )}
            {song?.playable && (
              <Button variant="outline" size="sm" onClick={() => playSongs([song], 0, song.id)}>
                <Play strokeWidth={1.75} />
                <Trans>Play the song</Trans>
              </Button>
            )}
          </>
        }
      />
      <p className="-mt-2 mb-2 max-w-[68ch] text-sub text-ink-muted">
        {videoPlays === true && (
          <Trans>
            Sona plays music videos in standard definition: Apple allows HD only on players with
            hardware protection.
          </Trans>
        )}
        {videoPlays === false && (
          <Trans>
            Sona can't play music videos here: the video opens in Apple Music in your browser.
          </Trans>
        )}
        {song && <> {songLine(song)}</>}
      </p>
      {q.data.notes && (
        <Shelf title={t`Notes`} className="mt-8">
          <Notes text={q.data.notes} />
        </Shelf>
      )}
      <CoverShelf
        title={t`More by ${artistName}`}
        aspect="video"
        items={q.data.moreByArtist.map((v) => videoTile(v, "year"))}
        more={
          video.artistId
            ? { to: "/artist/$id/view/$view", params: { id: video.artistId, view: "music-videos" } }
            : undefined
        }
      />
      <CoverShelf
        title={t`More like this`}
        aspect="video"
        items={q.data.moreInGenre.map((v) => videoTile(v))}
      />
      {video.releaseDate && (
        <footer className="mt-8 text-meta text-ink-muted num">
          <Trans>Released {releaseDate}</Trans>
        </footer>
      )}
    </>
  );
}
