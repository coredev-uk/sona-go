import { LibraryService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";
import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useCallback } from "react";

import { playSongs } from "@/features/player";
import { CoverRow } from "@/shared/components/CoverRow";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { Shelf } from "@/shared/components/Shelf";
import { keys } from "@/shared/lib/keys";
import { TrackList } from "@/shared/tracks/TrackList";

/** Apple's recently played songs, or this computer's play history when Apple can't be reached. */
function RecentlyPlayed() {
  const { t } = useLingui();
  const recent = useQuery({
    queryKey: keys.libraryRecent,
    queryFn: () => LibraryService.RecentlyPlayed(10, 0),
    retry: false,
  });
  const history = useQuery({
    queryKey: [...keys.libraryRecent, "local"],
    queryFn: () => LibraryService.History(30),
    enabled: recent.isError,
  });
  const songs =
    recent.data ??
    history.data?.days
      .flatMap((d) => d.entries)
      .slice(0, 10)
      .map((h) => h.song);
  const onPlay = useCallback(
    (i: number) => songs && playSongs(songs, i, "recently-played"),
    [songs],
  );

  if (recent.isPending || (recent.isError && history.isPending)) {
    return (
      <Shelf title={t`Recently played`}>
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
      <Shelf title={t`Recently played`}>
        <ErrorBlock
          title={t`Couldn't load what you played recently`}
          error={recent.error}
          onRetry={() => void recent.refetch()}
          retrying={recent.isRefetching}
        />
      </Shelf>
    );
  }
  if (!songs.length) {
    return (
      <Shelf title={t`Recently played`}>
        <EmptyState title={t`Nothing played yet`}>
          <Trans>Songs you play in Sona or Apple Music on any device appear here.</Trans>
        </EmptyState>
      </Shelf>
    );
  }
  return (
    <Shelf title={recent.isError ? t`Played on this computer` : t`Recently played`}>
      <TrackList
        songs={songs}
        columns={{ number: "row", artwork: true, artist: true, album: true }}
        label={t`Recently played songs`}
        onPlay={onPlay}
      />
    </Shelf>
  );
}

function RecentlyAdded() {
  const { t } = useLingui();
  const q = useQuery({
    queryKey: keys.libraryPage("albums", "dateAdded", true, -1),
    queryFn: () => LibraryService.Albums("dateAdded", true, 0, 12),
  });
  if (q.error) {
    return (
      <Shelf title={t`Recently added`}>
        <ErrorBlock
          title={t`Couldn't read your albums`}
          error={q.error}
          onRetry={() => void q.refetch()}
        />
      </Shelf>
    );
  }
  const items = q.data?.items;
  if (items && !items.length) return null;
  return (
    <Shelf title={t`Recently added`} more={{ to: "/library/albums" }}>
      <CoverRow
        label={t`Albums recently added to your library`}
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
  const { t } = useLingui();
  const q = useQuery({
    queryKey: keys.libraryPage("playlists", "lastModified", true, -1),
    queryFn: () => LibraryService.Playlists("lastModified", true, 0, 12),
  });
  const items = q.data?.items;
  if (q.error || (items && !items.length)) return null;
  return (
    <Shelf title={t`Your playlists`} more={{ to: "/library/playlists" }}>
      <CoverRow
        label={t`Recently updated playlists`}
        items={
          items?.map((p) => ({
            key: p.id,
            title: p.name,
            subtitle: p.trackCount
              ? plural(p.trackCount, { one: "# song", other: "# songs" })
              : p.curatorName,
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
