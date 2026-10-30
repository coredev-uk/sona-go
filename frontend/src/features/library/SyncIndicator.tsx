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
  if (current) {
    // One message per domain, so the noun is never spliced into the sentence.
    const domain = current.domain;
    const what =
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
    syncing = current.total ? `${what} · ${t`${done} of ${total}`}` : what;
  }
  const synced = lastSuccess ? relativeTime(lastSuccess) : "";

  return (
    <div className="mb-2 px-2.5" aria-live="polite">
      {running ? (
        <div>
          <p className="flex items-center gap-1.5 text-meta text-ink-muted">
            <RefreshCw aria-hidden className="size-3 animate-spin" strokeWidth={2} />
            <span className="truncate num">{syncing}</span>
          </p>
          <div
            role="progressbar"
            aria-label={t`Library sync`}
            aria-valuenow={pct}
            aria-valuemin={0}
            aria-valuemax={100}
            className="mt-1.5 h-0.5 overflow-hidden rounded-full bg-control-line/60"
          >
            <div
              className={cn("h-full bg-ink-muted transition-[width] duration-300")}
              style={{ width: `${pct}%` }}
            />
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
