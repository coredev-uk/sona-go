import { LibraryService } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { CircleAlert, RefreshCw } from "lucide-react";

import { hasAccount, useAuth } from "@/features/auth";
import { formatNumber, relativeTime } from "@/shared/lib/format";
import { run } from "@/shared/lib/run";
import { cn } from "@/shared/lib/utils";

import { useSyncStatus } from "./queries";
import { useSync } from "./sync";

/**
 * The library sync line at the foot of the sidebar: what is syncing and how
 * far, or when the library was last synced, with a Sync now action.
 *
 * Nothing here moves on its own: in WebKitGTK every frame the page draws
 * costs a composite of the whole window, so a spinning icon held a core
 * (docs/design.md, Motion). The count and the bar show the progress, and
 * move only when Go reports some.
 */
export function SyncIndicator() {
  const { t } = useLingui();
  const auth = useAuth();
  const status = useSyncStatus();
  const { running, current, last } = useSync();
  if (!hasAccount(auth.data?.state)) return null;

  const failed = last && !last.ok && last.errorCode !== "cancelled";
  const pct =
    current && current.total > 0
      ? Math.min(100, Math.round((current.done / current.total) * 100))
      : 0;
  const lastSuccess = last?.ok ? last.finishedAt : status.data?.lastSuccess;

  let syncing = t`Syncing your library`;
  let count = "";
  if (current) {
    // One message per domain, so the noun is never spliced into the sentence.
    const domain = current.domain;
    syncing =
      {
        artists: t`Syncing artists`,
        albums: t`Syncing albums`,
        songs: t`Syncing songs`,
        playlists: t`Syncing playlists`,
        "playlist-tracks": t`Syncing playlist songs`,
        ratings: t`Syncing loves`,
      }[domain] ?? t`Syncing ${domain}`;
    const done = formatNumber(current.done);
    const total = formatNumber(current.total);
    if (current.total) count = t`${done} of ${total}`;
  }
  const synced = lastSuccess ? relativeTime(lastSuccess) : "";

  return (
    <div className="mb-2 px-2.5" aria-live="polite">
      {running ? (
        <div>
          <p className="flex items-center gap-1.5 text-meta text-ink-muted">
            <RefreshCw aria-hidden className="size-3 shrink-0" strokeWidth={2} />
            <span className="truncate">{syncing}</span>
          </p>
          {/* The count sits beside the bar, so neither it nor the domain is
              cut off in the sidebar's width, even for a big library. The row
              keeps its height before the first count arrives. */}
          <div className="mt-1 flex h-4 items-center gap-2">
            <div
              role="progressbar"
              aria-label={t`Library sync`}
              aria-valuenow={pct}
              aria-valuetext={count || undefined}
              aria-valuemin={0}
              aria-valuemax={100}
              className="h-0.5 min-w-0 flex-1 overflow-hidden rounded-full bg-control-line/60"
            >
              <div className="h-full bg-ink-muted" style={{ width: `${pct}%` }} />
            </div>
            {count && <span className="shrink-0 text-meta text-ink-faint num">{count}</span>}
          </div>
        </div>
      ) : (
        <div className="flex items-center justify-between gap-2 text-meta">
          <span
            className={cn(
              "flex min-w-0 items-center gap-1.5",
              failed ? "text-ink" : "text-ink-faint",
            )}
          >
            {failed && (
              <CircleAlert aria-hidden className="size-3 shrink-0 text-danger" strokeWidth={2} />
            )}
            <span className="truncate" title={failed ? last.error : undefined}>
              {failed ? (
                <Trans>Library sync failed</Trans>
              ) : lastSuccess ? (
                <Trans>Synced {synced}</Trans>
              ) : (
                <Trans>Library not synced yet</Trans>
              )}
            </span>
          </span>
          <button
            type="button"
            onClick={() => void run(t`Couldn't start a library sync`, LibraryService.Sync())}
            className="shrink-0 rounded-sm text-ink-muted underline-offset-2 hover:text-ink hover:underline"
          >
            {failed ? <Trans>Retry</Trans> : <Trans>Sync now</Trans>}
          </button>
        </div>
      )}
    </div>
  );
}
