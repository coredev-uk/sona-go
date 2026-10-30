import { LibraryService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";
import { useCallback } from "react";

import { playSongs } from "@/features/player";
import { CoverRow } from "@/shared/components/CoverRow";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Shelf } from "@/shared/components/Shelf";
import { plural } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { TrackList } from "@/shared/tracks/TrackList";

/** Apple's recently played songs, or this computer's play history when Apple can't be reached. */
function RecentlyPlayed() {
  const recent = useQuery({
    queryKey: keys.libraryRecent,
    queryFn: () => LibraryService.RecentlyPlayed(10, 0),
    retry: false,
  });
  const history = useQuery({
    queryKey: [...keys.libraryRecent, "local"],
    queryFn: () => LibraryService.History(10),
    enabled: recent.isError,
  });
  const songs = recent.data ?? history.data?.map((h) => h.song);
  const onPlay = useCallback(
    (i: number) => songs && playSongs(songs, i, "recently-played"),
    [songs],
  );

  if (recent.isPending || (recent.isError && history.isPending)) {
    return (
      <Shelf title="Recently played">
        <div aria-busy className="flex flex-col">
          {Array.from({ length: 5 }, (_, i) => (
            <div key={i} className="flex h-11 items-center gap-3 px-2">
              <div className="ml-14 size-8 rounded-sm bg-hover" />
              <div className="h-3.5 rounded-sm bg-hover" style={{ width: `${24 + i * 5}%` }} />
            </div>
          ))}
        </div>
      </Shelf>
    );
  }
  if (!songs) {
    return (
      <Shelf title="Recently played">
        <ErrorBlock
          title="Couldn't load what you played recently"
          error={recent.error}
          onRetry={() => void recent.refetch()}
          retrying={recent.isRefetching}
        />
      </Shelf>
    );
  }
  if (!songs.length) {
    return (
      <Shelf title="Recently played">
        <EmptyState title="Nothing played yet">
          Songs you play in Sona or Apple Music on any device appear here.
        </EmptyState>
      </Shelf>
    );
  }
  return (
    <Shelf title={recent.isError ? "Played on this computer" : "Recently played"}>
      <TrackList
        songs={songs}
        columns={{ number: "row", artwork: true, artist: true, album: true }}
        label="Recently played songs"
        onPlay={onPlay}
      />
    </Shelf>
  );
}

function RecentlyAdded() {
  const q = useQuery({
    queryKey: keys.libraryPage("albums", "dateAdded", true, -1),
    queryFn: () => LibraryService.Albums("dateAdded", true, 0, 12),
  });
  if (q.error) {
    return (
      <Shelf title="Recently added">
        <ErrorBlock
          title="Couldn't read your albums"
          error={q.error}
          onRetry={() => void q.refetch()}
        />
      </Shelf>
    );
  }
  const items = q.data?.items;
  if (items && !items.length) return null;
  return (
    <Shelf title="Recently added" more={{ to: "/library/albums" }}>
      <CoverRow
        label="Albums recently added to your library"
        items={
          items?.map((a) => ({
            key: a.id,
            title: a.name,
            subtitle: a.artistName,
            art: a.art.medium,
            link: { to: "/album/$id", params: { id: a.id } },
          })) ?? []
        }
      />
    </Shelf>
  );
}

function RecentPlaylists() {
  const q = useQuery({
    queryKey: keys.libraryPage("playlists", "lastModified", true, -1),
    queryFn: () => LibraryService.Playlists("lastModified", true, 0, 12),
  });
  const items = q.data?.items;
  if (q.error || (items && !items.length)) return null;
  return (
    <Shelf title="Your playlists" more={{ to: "/library/playlists" }}>
      <CoverRow
        label="Recently updated playlists"
        items={
          items?.map((p) => ({
            key: p.id,
            title: p.name,
            subtitle: p.trackCount ? plural(p.trackCount, "song") : p.curatorName,
            art: p.art.medium,
            link: { to: "/playlist/$id", params: { id: p.id } },
          })) ?? []
        }
      />
    </Shelf>
  );
}

/**
 * What Sona knows without Apple's recommendations: recent plays (Apple's,
 * or this computer's history offline), recent additions and playlists.
 * Shown signed out, offline without a cached Home, or when Home is empty.
 */
export function LocalShelves({ signedIn }: { signedIn: boolean }) {
  return (
    <>
      {signedIn && <RecentlyPlayed />}
      <RecentlyAdded />
      <RecentPlaylists />
    </>
  );
}
