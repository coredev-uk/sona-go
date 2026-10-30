import { CatalogService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { ExternalLink, Play } from "lucide-react";

import { playSongs } from "@/features/player";
import { Badges } from "@/shared/components/Badges";
import { ArtistLinks } from "@/shared/components/ArtistLinks";
import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { CoverShelf } from "@/shared/components/CoverShelf";
import { DetailError, DetailSkeleton } from "@/shared/components/DetailStates";
import { Notes } from "@/shared/components/Notes";
import { Shelf } from "@/shared/components/Shelf";
import { formatTime, longDate, yearOf } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { run } from "@/shared/lib/run";
import { videoTile } from "@/shared/lib/tiles";
import { Button } from "@/shared/ui/button";

const route = getRouteApi("/music-video/$id");

/**
 * A music video's page. Sona plays audio only, so the video itself opens
 * in Apple Music on the web; the song it belongs to plays here.
 */
export function VideoPage() {
  const { id } = route.useParams();
  const q = useQuery({
    queryKey: keys.catalogVideo(id),
    queryFn: () => CatalogService.MusicVideo(id),
  });

  if (q.isPending) return <DetailSkeleton />;
  if (q.error) {
    return (
      <DetailError
        noun="music video"
        error={q.error}
        retry={() => void q.refetch()}
        retrying={q.isRefetching}
        back={{ label: "Go to Search", to: "/search" }}
      />
    );
  }
  const { video, song } = q.data;
  const meta = ["Music video", yearOf(video.releaseDate), formatTime(video.durationMs), video.genre]
    .filter(Boolean)
    .join(" · ");
  const formats = [video.has4K && "4K", video.hasHDR && "HDR"].filter((f): f is string => !!f);

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
            {video.url && (
              <Button
                size="sm"
                onClick={() =>
                  void run("Couldn't open Apple Music", CatalogService.OpenInAppleMusic(video.url))
                }
              >
                <ExternalLink strokeWidth={1.75} />
                Watch in Apple Music
              </Button>
            )}
            {song?.playable && (
              <Button variant="outline" size="sm" onClick={() => playSongs([song], 0, song.id)}>
                <Play strokeWidth={1.75} />
                Play the song
              </Button>
            )}
          </>
        }
      />
      <p className="-mt-2 mb-2 max-w-[68ch] text-sub text-ink-muted">
        Sona plays music, not video: the video opens in Apple Music in your browser.
        {song && (
          <>
            {" "}
            The song is{" "}
            <Link
              to="/song/$id"
              params={{ id: song.catalogId || song.id }}
              className="text-ink underline-offset-2 hover:underline"
            >
              {song.name}
            </Link>
            {song.albumName && <> from {song.albumName}</>}.
          </>
        )}
      </p>
      {q.data.notes && (
        <Shelf title="Notes" className="mt-8">
          <Notes text={q.data.notes} />
        </Shelf>
      )}
      <CoverShelf
        title={`More by ${video.artistName}`}
        aspect="video"
        items={q.data.moreByArtist.map((v) => videoTile(v, "year"))}
        more={
          video.artistId
            ? { to: "/artist/$id/view/$view", params: { id: video.artistId, view: "music-videos" } }
            : undefined
        }
      />
      <CoverShelf
        title="More like this"
        aspect="video"
        items={q.data.moreInGenre.map((v) => videoTile(v))}
      />
      {video.releaseDate && (
        <footer className="mt-8 text-meta text-ink-muted num">
          Released {longDate(video.releaseDate)}
        </footer>
      )}
    </>
  );
}
