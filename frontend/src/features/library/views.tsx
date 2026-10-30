import type { LibraryEntry } from "@bindings/services";
import { useCallback } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { playSongs, shuffleSongs } from "@/features/player";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { CoverGrid, type CoverItem } from "@/shared/components/CoverGrid";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { formatTime, plural } from "@/shared/lib/format";
import { TrackList } from "@/shared/tracks/TrackList";

import { signIn, skeletonRows, skeletonTiles, useLibraryGate } from "./gate";
import { useFavoriteSongs, useMadeForYou, useMusicVideos, useRecentlyAdded } from "./queries";

function entryTile(e: LibraryEntry): CoverItem | undefined {
  if (e.album) {
    const a = e.album;
    return {
      key: a.id,
      title: a.name,
      subtitle: a.artistName,
      art: a.art.medium,
      explicit: a.explicit,
      link: { to: "/album/$id", params: { id: a.id } },
      menu: <ActionMenu kind="album" target={a} name={a.name} />,
    };
  }
  if (e.playlist) {
    const p = e.playlist;
    return {
      key: p.id,
      title: p.name,
      subtitle: p.curatorName || "Playlist",
      art: p.art.medium,
      link: { to: "/playlist/$id", params: { id: p.id } },
      menu: <ActionMenu kind="playlist" target={p} name={p.name} />,
    };
  }
  return undefined;
}

/** Albums and playlists in the order they joined the library, newest first. */
export function RecentlyAddedPage() {
  const list = useRecentlyAdded();
  const gate = useLibraryGate({
    total: list.total,
    pending: list.isPending,
    error: list.error,
    retry: () => void list.refetch(),
    retrying: list.isRefetching,
    noun: "recent additions",
    empty: "Albums and playlists you add in Apple Music appear here, newest first.",
    frame: "covers",
  });
  const items = list.isPending ? skeletonTiles : list.rows.map((e) => e && entryTile(e));
  return (
    <>
      <PageHeader
        title="Recently added"
        subline={
          list.total ? plural(list.total, "album or playlist", "albums and playlists") : undefined
        }
      />
      {gate ?? (
        <CoverGrid items={items} label="Recently added to your library" onRange={list.onRange} />
      )}
    </>
  );
}

/** Library songs rated Favourite, most recently favourited first. */
export function FavoriteSongsPage() {
  const list = useFavoriteSongs();
  const auth = useAuth();
  const play = useCallback((i: number) => playSongs(list.rows, i, "Favourite songs"), [list.rows]);
  const signedOut = auth.data && !hasAccount(auth.data.state) && auth.data.state !== "needs_reauth";
  let body;
  if (list.error) {
    body = (
      <ErrorBlock
        title="Couldn't read your favourite songs from this computer"
        error={list.error}
        onRetry={() => void list.refetch()}
        retrying={list.isRefetching}
      />
    );
  } else if (!list.isPending && list.total === 0) {
    body = signedOut ? (
      <EmptyState title="Sign in to see your favourite songs" action={signIn}>
        Songs you favourite in Sona or any Apple Music app are collected here.
      </EmptyState>
    ) : (
      <EmptyState title="No favourite songs yet">
        Choose Favourite on any song, here or in Apple Music, and it is kept here and in your
        library.
      </EmptyState>
    );
  } else {
    body = (
      <TrackList
        songs={list.isPending ? skeletonRows : list.rows}
        columns={{ number: "row", artwork: true, artist: true, album: true }}
        label="Favourite songs"
        onPlay={play}
        onRange={list.onRange}
      />
    );
  }
  return (
    <>
      <PageHeader
        title="Favourite songs"
        subline={list.total ? plural(list.total, "song") : undefined}
        actions={
          <PlayButtons
            label="your favourite songs"
            disabled={!list.total}
            onPlay={() => play(0)}
            onShuffle={() => void shuffleSongs(list.rows, "Favourite songs")}
          />
        }
      />
      {body}
    </>
  );
}

/** Online-only views: their sign-in, error and empty states. */
function onlineGate(opts: {
  signedOut: boolean;
  error: unknown;
  retry: () => void;
  retrying: boolean;
  noun: string;
  signInText: string;
  empty: { title: string; text: string } | null;
  /** Empty frames: cover slots for square art, a box for 16:9 videos. */
  frame: "covers" | "box";
}) {
  if (opts.signedOut) {
    return (
      <EmptyState frame={opts.frame} title={`Sign in to see your ${opts.noun}`} action={signIn}>
        {opts.signInText}
      </EmptyState>
    );
  }
  if (opts.error) {
    return (
      <ErrorBlock
        title={`Couldn't load your ${opts.noun} from Apple Music`}
        error={opts.error}
        onRetry={opts.retry}
        retrying={opts.retrying}
      />
    );
  }
  if (opts.empty) {
    return (
      <EmptyState frame={opts.frame} title={opts.empty.title}>
        {opts.empty.text}
      </EmptyState>
    );
  }
  return null;
}

function useSignedOut() {
  const auth = useAuth();
  return !!auth.data && !hasAccount(auth.data.state) && auth.data.state !== "needs_reauth";
}

/** The personal mixes Apple makes for the account, as library playlists. */
export function MadeForYouPage() {
  const q = useMadeForYou();
  const signedOut = useSignedOut();
  const gate = onlineGate({
    signedOut,
    error: q.error,
    retry: () => void q.refetch(),
    retrying: q.isRefetching,
    noun: "mixes",
    frame: "covers",
    signInText: "Apple Music makes personal mixes from what you play. Sign in to see yours.",
    empty:
      q.data && q.data.length === 0
        ? {
            title: "No mixes yet",
            text: "Apple Music makes mixes once it knows what you like. Keep listening and they appear here.",
          }
        : null,
  });
  const items: (CoverItem | undefined)[] = q.data
    ? q.data.map((p) => ({
        key: p.id,
        title: p.name,
        // The library copy's dates are when it was added, not when Apple last refreshed it.
        subtitle: p.curatorName || "Apple Music",
        art: p.art.medium,
        link: { to: "/playlist/$id", params: { id: p.id } },
        menu: <ActionMenu kind="playlist" target={p} name={p.name} />,
      }))
    : skeletonTiles.slice(0, 6);
  return (
    <>
      <PageHeader
        title="Made for you"
        subline={q.data?.length ? plural(q.data.length, "mix", "mixes") : undefined}
      />
      {gate ?? <CoverGrid items={items} label="Mixes made for you" />}
    </>
  );
}

/** Music videos in the library; each opens its page (video playback is not supported). */
export function MusicVideosPage() {
  const q = useMusicVideos();
  const signedOut = useSignedOut();
  const gate = onlineGate({
    signedOut,
    error: q.error,
    retry: () => void q.refetch(),
    retrying: q.isRefetching,
    noun: "music videos",
    frame: "box",
    signInText: "Music videos you add in Apple Music are listed here after you sign in.",
    empty:
      q.data && q.data.length === 0
        ? {
            title: "No music videos in your library",
            text: "Music videos you add in Apple Music on any device are listed here.",
          }
        : null,
  });
  const items: (CoverItem | undefined)[] = q.data
    ? q.data.map((v) => ({
        key: v.id,
        title: v.name,
        subtitle: [v.artistName, v.durationMs ? formatTime(v.durationMs) : ""]
          .filter(Boolean)
          .join(" · "),
        art: v.art.medium,
        aspect: "video",
        explicit: v.explicit,
        link: { to: "/music-video/$id", params: { id: v.catalogId || v.id } },
      }))
    : skeletonTiles.slice(0, 6);
  return (
    <>
      <PageHeader
        title="Music videos"
        subline={q.data?.length ? plural(q.data.length, "music video") : undefined}
      />
      {gate ?? (
        <CoverGrid items={items} label="Music videos in your library" min={240} aspect="video" />
      )}
    </>
  );
}
