import type { CDMStatus } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { CircleAlert, CircleCheck, CircleSlash, LoaderCircle, RotateCw } from "lucide-react";
import { useRef, useState, type ReactNode } from "react";

import {
  canRetryCdm,
  cdmFailedState,
  cdmPercent,
  cdmProblem,
  cdmProgress,
  retryCdm,
  useCdm,
} from "@/features/cdm";
import { useCapabilities } from "@/shared/lib/capabilities";
import { formatBytes } from "@/shared/lib/format";
import { Button } from "@/shared/ui/button";

import { Well } from "./parts";
import { widevineView } from "./steps";

const ICON_MUTED = "size-4 shrink-0 text-ink-muted";
const ICON_POSITIVE = "size-4 shrink-0 text-positive";
const ICON_DANGER = "size-4 shrink-0 text-danger";
const ICON_SPIN = "size-4 shrink-0 animate-spin text-ink-muted motion-reduce:animate-none";

/** The download's bar: the value is the bar's own, the figures beside it decoration. */
function DownloadBar({ status }: { status: CDMStatus }) {
  const { t } = useLingui();
  const fraction = cdmProgress(status);
  const percent = cdmPercent(status);
  const received = formatBytes(status.received);
  const total = formatBytes(status.total);
  return (
    <div className="mt-2.5 flex items-center gap-3 ps-6">
      <div
        role="progressbar"
        aria-label={t`Widevine download`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={fraction === null ? undefined : Math.round(fraction * 100)}
        aria-valuetext={fraction === null ? undefined : t`${percent}, ${received} of ${total}`}
        className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-ink-faint/30"
      >
        {fraction !== null && (
          <div
            className="h-full rounded-full bg-ink-muted transition-[width] duration-[120ms]"
            style={{ width: `${fraction * 100}%` }}
          />
        )}
      </div>
      <span aria-hidden className="shrink-0 text-meta text-ink-faint num">
        {fraction === null ? <Trans>Starting…</Trans> : t`${percent} · ${received} of ${total}`}
      </span>
    </div>
  );
}

/**
 * Widevine at the head of the Playback step, in one block: its state (an
 * <output>, so a change is announced once), the download's bar while it
 * runs, one line on what it means, and Retry after a failure Sona can
 * retry. The download is Sona's own, started after the first paint (no
 * consent step). Without playback on this platform it says so; where the
 * engine needs no CDM it says nothing.
 */
export function WidevineStatus() {
  const { t } = useLingui();
  const caps = useCapabilities();
  const status = useCdm((s) => s.status);
  const view = widevineView(caps, status);
  const [retrying, setRetrying] = useState(false);
  const stateRef = useRef<HTMLOutputElement>(null);
  if (view === "not-needed") return null;

  const onRetry = async () => {
    setRetrying(true);
    stateRef.current?.focus();
    await retryCdm();
    setRetrying(false);
  };

  const version = status?.version ?? "";
  const what = t`Google's component that plays Apple Music's protected songs. Sona downloads it, checks Google's signature and keeps it up to date.`;
  let icon: ReactNode = null;
  let state: ReactNode = <span className="block h-3 w-40 rounded-sm bg-hover" />;
  let detail: ReactNode = what;
  switch (view) {
    case "unavailable":
      icon = <CircleSlash aria-hidden className={ICON_MUTED} strokeWidth={1.75} />;
      state = <Trans>Playback isn't available on this platform yet</Trans>;
      detail = <Trans>Browsing Apple Music and your library works as usual.</Trans>;
      break;
    case "unsupported":
      icon = <CircleSlash aria-hidden className={ICON_MUTED} strokeWidth={1.75} />;
      state = <Trans>Playback isn't available on this computer</Trans>;
      detail = cdmProblem("unsupported_platform");
      break;
    case "waiting":
      icon = <LoaderCircle aria-hidden className={ICON_SPIN} strokeWidth={1.75} />;
      state = <Trans>Starting Widevine's download…</Trans>;
      break;
    case "downloading":
      icon = <LoaderCircle aria-hidden className={ICON_SPIN} strokeWidth={1.75} />;
      state = <Trans>Downloading Widevine…</Trans>;
      detail = <Trans>Songs play once it's ready. The download carries on if you continue.</Trans>;
      break;
    case "ready":
      icon = <CircleCheck aria-hidden className={ICON_POSITIVE} strokeWidth={1.75} />;
      state = version ? (
        <Trans>Widevine {version} is ready</Trans>
      ) : (
        <Trans>Widevine is ready</Trans>
      );
      break;
    case "failed":
      icon = <CircleAlert aria-hidden className={ICON_DANGER} strokeWidth={1.75} />;
      state = status ? cdmFailedState(status.code) : null;
      detail = status ? cdmProblem(status.code) : null;
      break;
  }
  const retry = view === "failed" && canRetryCdm(status);

  return (
    <Well>
      <div className="flex items-center gap-3">
        <output
          ref={stateRef}
          tabIndex={-1}
          aria-describedby={detail ? "setup-widevine-detail" : undefined}
          className="flex min-w-0 flex-1 items-center gap-2 text-ui font-medium text-ink focus:outline-none"
        >
          {icon}
          <span>{state}</span>
        </output>
        {retry && (
          <Button variant="outline" size="sm" disabled={retrying} onClick={() => void onRetry()}>
            <RotateCw strokeWidth={1.75} />
            {retrying ? <Trans>Retrying…</Trans> : <Trans>Retry</Trans>}
          </Button>
        )}
      </div>
      {view === "downloading" && status && <DownloadBar status={status} />}
      {detail && (
        <p
          id="setup-widevine-detail"
          className="mt-1 max-w-[60ch] ps-6 text-sub text-pretty text-ink-muted"
        >
          {detail}
          {retry && (
            <>
              {" "}
              <Trans>Sona also tries again by itself later.</Trans>
            </>
          )}
        </p>
      )}
    </Well>
  );
}
