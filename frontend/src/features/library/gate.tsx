import type { ReactNode } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";

import { useSyncStatus } from "./queries";
import { useSync } from "./sync";

export const signIn = {
  label: "Sign in",
  to: "/settings",
  search: { section: "account" },
} as const;

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
  noun: string;
  empty: string;
  frame: "box" | "covers";
}): ReactNode {
  const auth = useAuth();
  const syncing = useSync((s) => s.running);
  const status = useSyncStatus();
  if (opts.error) {
    return (
      <ErrorBlock
        title={`Couldn't read your ${opts.noun} from this computer`}
        error={opts.error}
        onRetry={opts.retry}
        retrying={opts.retrying}
      />
    );
  }
  if (opts.pending || opts.total > 0 || !auth.data) return null;
  const state = auth.data.state;
  if (state === "loading") return null;
  if (!hasAccount(state) && state !== "needs_reauth") {
    return (
      <EmptyState frame={opts.frame} title={`Sign in to see your ${opts.noun}`} action={signIn}>
        Sona copies your Apple Music library to this computer after you sign in, so it opens
        instantly and works offline.
      </EmptyState>
    );
  }
  if (syncing || !status.data?.lastSuccess) {
    return (
      <EmptyState frame={opts.frame} title="Your library is syncing">
        Your {opts.noun} appear here as Sona copies them from Apple Music. Large libraries take a
        minute or two the first time.
      </EmptyState>
    );
  }
  return (
    <EmptyState frame={opts.frame} title={`No ${opts.noun} in your library yet`}>
      {opts.empty}
    </EmptyState>
  );
}

/** Loading rows and tiles at the lists' real sizes. */
export const skeletonRows: undefined[] = Array.from({ length: 16 });
export const skeletonTiles: undefined[] = Array.from({ length: 12 });
