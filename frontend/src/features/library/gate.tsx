import { Trans, useLingui } from "@lingui/react/macro";
import type { ReactNode } from "react";

import { isSignedOut, useAuth } from "@/features/auth";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";

import { useSyncStatus } from "./queries";
import { signInAction } from "./sign-in";
import { useSync } from "./sync";

type LibraryKind = "songs" | "albums" | "artists" | "playlists" | "recent";

/**
 * What a library page shows instead of its list: an error, a sign-in
 * prompt, a first sync in progress, or an empty library. `null` means show
 * the list (or its skeleton while the first page loads).
 */
export function useLibraryGate(opts: {
  total: number;
  pending: boolean;
  error: unknown;
  retry: () => void;
  retrying: boolean;
  kind: LibraryKind;
  empty: string;
  frame: "box" | "covers";
}): ReactNode {
  const { t } = useLingui();
  const auth = useAuth();
  const syncing = useSync((s) => s.running);
  const status = useSyncStatus();
  // One full sentence per kind, so translations never splice a noun into a sentence.
  const copy = (() => {
    switch (opts.kind) {
      case "songs":
        return {
          error: t`Couldn't read your songs from this computer`,
          signIn: t`Sign in to see your songs`,
          syncing: t`Your songs appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time.`,
          none: t`No songs in your library yet`,
        };
      case "albums":
        return {
          error: t`Couldn't read your albums from this computer`,
          signIn: t`Sign in to see your albums`,
          syncing: t`Your albums appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time.`,
          none: t`No albums in your library yet`,
        };
      case "artists":
        return {
          error: t`Couldn't read your artists from this computer`,
          signIn: t`Sign in to see your artists`,
          syncing: t`Your artists appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time.`,
          none: t`No artists in your library yet`,
        };
      case "playlists":
        return {
          error: t`Couldn't read your playlists from this computer`,
          signIn: t`Sign in to see your playlists`,
          syncing: t`Your playlists appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time.`,
          none: t`No playlists in your library yet`,
        };
      case "recent":
        return {
          error: t`Couldn't read your recent additions from this computer`,
          signIn: t`Sign in to see your recent additions`,
          syncing: t`Your recent additions appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time.`,
          none: t`No recent additions in your library yet`,
        };
    }
  })();
  if (opts.error) {
    return (
      <ErrorBlock
        title={copy.error}
        error={opts.error}
        onRetry={opts.retry}
        retrying={opts.retrying}
      />
    );
  }
  if (opts.pending || opts.total > 0 || !auth.data) return null;
  const state = auth.data.state;
  if (state === "restoring") return null;
  if (isSignedOut(state)) {
    return (
      <EmptyState frame={opts.frame} title={copy.signIn} action={signInAction()}>
        <Trans>
          Sona copies your Apple Music library to this computer after you sign in, so it opens
          instantly and works offline.
        </Trans>
      </EmptyState>
    );
  }
  if (syncing || !status.data?.lastSuccess) {
    return (
      <EmptyState frame={opts.frame} title={t`Your library is syncing`}>
        {copy.syncing}
      </EmptyState>
    );
  }
  return (
    <EmptyState frame={opts.frame} title={copy.none}>
      {opts.empty}
    </EmptyState>
  );
}

/** Loading rows and tiles at the lists' real sizes. */
export const skeletonRows: undefined[] = Array.from({ length: 16 });
export const skeletonTiles: undefined[] = Array.from({ length: 12 });
