import type { LibraryEntry } from "@bindings/services";
import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useCallback } from "react";

import { isSignedOut, useAuth } from "@/features/auth";
import { playSongs, shuffleSongs } from "@/features/player";
import { CoverGrid, type CoverItem } from "@/shared/components/CoverGrid";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { formatTime } from "@/shared/lib/format";
import { videoTile } from "@/shared/lib/tiles";
import { TrackList } from "@/shared/tracks/TrackList";

import { signInAction } from "./sign-in";
import { skeletonRows, skeletonTiles, useLibraryGate } from "./gate";
import { useFavoriteSongs, useMusicVideos, useRecentlyAdded } from "./queries";

/** `playlist` is the fallback subtitle for playlists without a curator. */
function entryTile(e: LibraryEntry, playlist: string): CoverItem | undefined {
  if (e.album) {
    const a = e.album;
    return {
      key: a.id,
      title: a.name,
      subtitle: a.artistName,
      art: a.art.medium,
      explicit: a.explicit,
      link: { to: "/album/$id", params: { id: a.id } },
      target: { kind: "album", target: a },
    };
  }
  if (e.playlist) {
    const p = e.playlist;
    return {
      key: p.id,
      title: p.name,
      subtitle: p.curatorName || playlist,
      art: p.art.medium,
      link: { to: "/playlist/$id", params: { id: p.id } },
      target: { kind: "playlist", target: p },
    };
  }
  return undefined;
}

/** Albums and playlists in the order they joined the library, newest first. */
export function RecentlyAddedPage() {
  const { t } = useLingui();
  const list = useRecentlyAdded();
  const gate = useLibraryGate({
    total: list.total,
    pending: list.isPending,
    error: list.error,
    retry: () => void list.refetch(),
    retrying: list.isRefetching,
    kind: "recent",
    empty: t`Albums and playlists you add in Apple Music appear here, newest first.`,
    frame: "covers",
  });
  const items = list.isPending
    ? skeletonTiles
    : list.rows.map((e) => e && entryTile(e, t`Playlist`));
  return (
    <>
      <PageHeader
        title={t`Recently added`}
        subline={
          list.total
            ? plural(list.total, { one: "# album or playlist", other: "# albums and playlists" })
            : undefined
        }
      />
      {gate ?? <CoverGrid items={items} label={t`Recently added`} onRange={list.onRange} />}
    </>
  );
}

/** Library songs rated Favourite, most recently favourited first. */
export function FavoriteSongsPage() {
  const { t } = useLingui();
  const list = useFavoriteSongs();
  const auth = useAuth();
  const play = useCallback(
    (i: number) => playSongs(list.rows, i, t`Favourite songs`),
    [list.rows, t],
  );
  const signedOut = isSignedOut(auth.data?.state);
  let body;
  if (list.error) {
    body = (
      <ErrorBlock
        title={t`Couldn't read your favourite songs from this computer`}
        error={list.error}
        onRetry={() => void list.refetch()}
        retrying={list.isRefetching}
      />
    );
  } else if (!list.isPending && list.total === 0) {
    body = signedOut ? (
      <EmptyState
        title={t`You need to sign in to see your favourite songs`}
        action={signInAction()}
      >
        <Trans>Songs you favourite in Sona or any Apple Music app are collected here.</Trans>
      </EmptyState>
    ) : (
      <EmptyState title={t`No favourite songs yet`}>
        <Trans>
          Choose Favourite on any song, here or in Apple Music, and it is kept here and in your
          library.
        </Trans>
      </EmptyState>
    );
  } else {
    body = (
      <TrackList
        songs={list.isPending ? skeletonRows : list.rows}
        columns={{ number: "row", artwork: true, artist: true, album: true }}
        label={t`Favourite songs`}
        onPlay={play}
        onRange={list.onRange}
      />
    );
  }
  return (
    <>
      <PageHeader
        title={t`Favourite songs`}
        subline={list.total ? plural(list.total, { one: "# song", other: "# songs" }) : undefined}
        actions={
          <PlayButtons
            playLabel={t`Play your favourite songs`}
            disabled={!list.total}
            onPlay={() => play(0)}
            onShuffle={() => void shuffleSongs(list.rows, t`Favourite songs`)}
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
  signInTitle: string;
  errorTitle: string;
  signInText: string;
  empty: { title: string; text: string } | null;
  /** Empty frames: cover slots for square art, a box for 16:9 videos. */
  frame: "covers" | "box";
}) {
  if (opts.signedOut) {
    return (
      <EmptyState frame={opts.frame} title={opts.signInTitle} action={signInAction()}>
        {opts.signInText}
      </EmptyState>
    );
  }
  if (opts.error) {
    return (
      <ErrorBlock
        title={opts.errorTitle}
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
  return isSignedOut(auth.data?.state);
}

/** Music videos in the library; each opens its page, and plays where videos play. */
export function MusicVideosPage() {
  const { t } = useLingui();
  const q = useMusicVideos();
  const signedOut = useSignedOut();
  const gate = onlineGate({
    signedOut,
    error: q.error,
    retry: () => void q.refetch(),
    retrying: q.isRefetching,
    signInTitle: t`Sign in to see your music videos`,
    errorTitle: t`Couldn't load your music videos from Apple Music`,
    frame: "box",
    signInText: t`Music videos you add in Apple Music are listed here after you sign in.`,
    empty:
      q.data && q.data.length === 0
        ? {
            title: t`No music videos in your library`,
            text: t`Music videos you add in Apple Music on any device are listed here.`,
          }
        : null,
  });
  const items: (CoverItem | undefined)[] = q.data
    ? q.data.map((v) => ({
        ...videoTile(v),
        subtitle: [v.artistName, v.durationMs ? formatTime(v.durationMs) : ""]
          .filter(Boolean)
          .join(" · "),
      }))
    : skeletonTiles.slice(0, 6);
  return (
    <>
      <PageHeader
        title={t`Music videos`}
        subline={
          q.data?.length
            ? plural(q.data.length, { one: "# music video", other: "# music videos" })
            : undefined
        }
      />
      {gate ?? (
        <CoverGrid items={items} label={t`Music videos in your library`} min={240} aspect="video" />
      )}
    </>
  );
}
