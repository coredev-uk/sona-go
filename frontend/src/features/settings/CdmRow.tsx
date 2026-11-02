import type { CDMStatus } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { CircleAlert, CircleCheck, RotateCw } from "lucide-react";
import { useRef, useState } from "react";

import {
  canRetryCdm,
  cdmFailedState,
  cdmPercent,
  cdmProblem,
  cdmProgress,
  retryCdm,
  useCdm,
} from "@/features/cdm";
import { formatBytes, relativeTime } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";

import { ROW_FRAME } from "./SettingRow";

/** A download's bar: the value is the bar's own (role progressbar), the text beside it is decoration. */
function DownloadProgress({ status, label }: { status: CDMStatus; label: string }) {
  const { t } = useLingui();
  const fraction = cdmProgress(status);
  const percent = cdmPercent(status);
  const received = formatBytes(status.received);
  const total = formatBytes(status.total);
  return (
    <div className="flex w-44 flex-col gap-1">
      <div
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={fraction === null ? undefined : Math.round(fraction * 100)}
        aria-valuetext={fraction === null ? undefined : t`${percent}, ${received} of ${total}`}
        className="h-1 w-full overflow-hidden rounded-full bg-ink-faint/40"
      >
        {fraction !== null && (
          <div
            className="h-full rounded-full bg-ink-muted transition-[width] duration-[120ms]"
            style={{ width: `${fraction * 100}%` }}
          />
        )}
      </div>
      {fraction !== null && (
        <span aria-hidden className="text-right text-meta text-ink-faint">
          {t`${percent} · ${received} of ${total}`}
        </span>
      )}
    </div>
  );
}

/**
 * Settings → Playback: the Widevine CDM Sona downloads from Google, its version, a running download's progress,
 * and Retry when the download or the last update check failed. The state is
 * an <output> named by the row's label, so a change is announced once; the
 * progress is the bar's value, never a live region.
 */
export function CdmRow() {
  const { t } = useLingui();
  const status = useCdm((s) => s.status);
  const [retrying, setRetrying] = useState(false);
  const stateRef = useRef<HTMLOutputElement>(null);

  const failed = status?.state === "failed";
  const ready = status?.state === "ready";
  const retry = canRetryCdm(status);
  const problem = status?.code ? cdmProblem(status.code) : "";
  const version = status?.version ?? "";
  const checked = status?.checkedAt ? relativeTime(status.checkedAt) : "";

  // The button goes away while the download runs, so focus moves to the
  // state it starts (not to the page) and Tab reaches Retry again if it fails.
  const onRetry = async () => {
    setRetrying(true);
    stateRef.current?.focus();
    await retryCdm();
    setRetrying(false);
  };

  let state = <span className="block h-3 w-32 rounded-sm bg-hover" />;
  if (status && failed) {
    state = (
      <>
        <CircleAlert aria-hidden className="size-4 shrink-0 text-danger" strokeWidth={1.75} />
        <span className="font-medium">{cdmFailedState(status.code)}</span>
      </>
    );
  } else if (status && ready) {
    state = (
      <>
        <CircleCheck aria-hidden className="size-4 shrink-0 text-positive" strokeWidth={1.75} />
        <span>{version ? <Trans>Version {version}</Trans> : <Trans>Ready</Trans>}</span>
      </>
    );
  } else if (status?.state === "downloading") {
    state = (
      <span>
        <Trans>Downloading…</Trans>
      </span>
    );
  } else if (status) {
    state = (
      <span className="text-ink-muted">
        <Trans>Not downloaded yet</Trans>
      </span>
    );
  }

  // The line under the state: what is happening to an installed copy.
  let note = null;
  if (ready && status?.updating) {
    note = <Trans>Downloading an update…</Trans>;
  } else if (ready && status?.code) {
    note = <Trans>Couldn't check for updates</Trans>;
  } else if (ready && checked) {
    note = <Trans>Checked for updates {checked}</Trans>;
  }

  const downloading = status?.state === "downloading" || (ready && !!status?.updating);

  return (
    <div data-setting="cdm" className={cn(ROW_FRAME, "flex items-start justify-between gap-10")}>
      <div className="min-w-0">
        <label htmlFor="cdm" id="cdm-label" className="text-ui font-medium text-ink">
          <Trans>Widevine</Trans>
        </label>
        <p id="cdm-desc" className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">
          <Trans>
            Google's Widevine plays Apple Music's protected songs. Sona downloads it from Google and
            keeps it up to date by itself; an update applies the next time playback starts.
          </Trans>
        </p>
        {problem && (
          <p id="cdm-problem" className="mt-2 max-w-[56ch] text-sub text-ink">
            {problem}
            {retry && (
              <>
                {" "}
                <Trans>Sona also tries again by itself later.</Trans>
              </>
            )}
          </p>
        )}
      </div>
      <div className="flex w-44 shrink-0 flex-col items-end gap-1.5 pt-0.5">
        <output
          ref={stateRef}
          id="cdm"
          tabIndex={-1}
          aria-describedby={problem ? "cdm-desc cdm-problem" : "cdm-desc"}
          className="flex items-center gap-1.5 text-sub text-ink"
        >
          {state}
        </output>
        {note && <span className="text-right text-meta text-ink-faint">{note}</span>}
        {downloading && status && (
          <DownloadProgress
            status={status}
            label={status.updating ? t`Widevine update` : t`Widevine download`}
          />
        )}
        {retry && (
          <Button
            variant="outline"
            size="sm"
            className="mt-1"
            aria-describedby="cdm-problem"
            disabled={retrying}
            onClick={() => void onRetry()}
          >
            <RotateCw strokeWidth={1.75} />
            {retrying ? <Trans>Retrying…</Trans> : <Trans>Retry</Trans>}
          </Button>
        )}
      </div>
    </div>
  );
}
