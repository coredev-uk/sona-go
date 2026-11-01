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

import { StepActions, StepHeader } from "./parts";
import { widevineView } from "./steps";

const ICON_MUTED = "size-4 shrink-0 text-ink-muted";
const ICON_POSITIVE = "size-4 shrink-0 text-positive";
const ICON_DANGER = "size-4 shrink-0 text-danger";
const ICON_SPIN = "size-4 shrink-0 animate-spin text-ink-muted motion-reduce:animate-none";

/** The download's bar across the panel: the value is the bar's own, the figures beside it decoration. */
function DownloadBar({ status }: { status: CDMStatus }) {
  const { t } = useLingui();
  const fraction = cdmProgress(status);
  const percent = cdmPercent(status);
  const received = formatBytes(status.received);
  const total = formatBytes(status.total);
  return (
    <div className="mt-4 flex flex-col gap-1.5">
      <div
        role="progressbar"
        aria-label={t`Widevine download`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={fraction === null ? undefined : Math.round(fraction * 100)}
        aria-valuetext={fraction === null ? undefined : t`${percent}, ${received} of ${total}`}
        className="h-1.5 w-full overflow-hidden rounded-full bg-ink-faint/30"
      >
        {fraction !== null && (
          <div
            className="h-full rounded-full bg-ink-muted transition-[width] duration-[120ms]"
            style={{ width: `${fraction * 100}%` }}
          />
        )}
      </div>
      <span aria-hidden className="text-meta text-ink-faint num">
        {fraction === null ? (
          <Trans>Starting the download…</Trans>
        ) : (
          t`${percent} · ${received} of ${total}`
        )}
      </span>
    </div>
  );
}

/**
 * The Widevine step: what Widevine is and where it comes from, then the
 * download Sona started after the first paint (no consent step: Sona
 * downloads it by itself, as it does on every start). The state is an
 * <output>, so a change is announced once; the progress is the bar's value.
 */
export function WidevineStep({ onNext }: { onNext: () => void }) {
  const { t } = useLingui();
  const caps = useCapabilities();
  const status = useCdm((s) => s.status);
  const view = widevineView(caps, status);
  const [retrying, setRetrying] = useState(false);
  const stateRef = useRef<HTMLOutputElement>(null);

  const onRetry = async () => {
    setRetrying(true);
    stateRef.current?.focus();
    await retryCdm();
    setRetrying(false);
  };

  const version = status?.version ?? "";
  let icon: ReactNode = null;
  let state: ReactNode = <span className="block h-3 w-40 rounded-sm bg-hover" />;
  let detail: ReactNode = null;
  switch (view) {
    case "unavailable":
      icon = <CircleSlash aria-hidden className={ICON_MUTED} strokeWidth={1.75} />;
      state = <Trans>Playback isn't available on this platform yet</Trans>;
      detail = <Trans>Browsing Apple Music and your library works as usual.</Trans>;
      break;
    case "not-needed":
      icon = <CircleCheck aria-hidden className={ICON_POSITIVE} strokeWidth={1.75} />;
      state = <Trans>Nothing to download on this computer</Trans>;
      break;
    case "unsupported":
      icon = <CircleSlash aria-hidden className={ICON_MUTED} strokeWidth={1.75} />;
      state = <Trans>Playback isn't available on this computer</Trans>;
      detail = cdmProblem("unsupported_platform");
      break;
    case "waiting":
      icon = <LoaderCircle aria-hidden className={ICON_SPIN} strokeWidth={1.75} />;
      state = <Trans>Starting the download…</Trans>;
      break;
    case "downloading":
      icon = <LoaderCircle aria-hidden className={ICON_SPIN} strokeWidth={1.75} />;
      state = <Trans>Downloading…</Trans>;
      detail = <Trans>The download carries on if you continue.</Trans>;
      break;
    case "ready":
      icon = <CircleCheck aria-hidden className={ICON_POSITIVE} strokeWidth={1.75} />;
      state = version ? <Trans>Version {version} is ready</Trans> : <Trans>Ready</Trans>;
      detail = (
        <Trans>
          Sona checks for a newer version every day; an update applies the next time playback
          starts.
        </Trans>
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
    <>
      <StepHeader title={t`Widevine`}>
        <Trans>
          Apple Music's songs are protected, and Google's Widevine is what plays them. Sona
          downloads it from Google, checks Google's signature and keeps it up to date by itself.
        </Trans>
      </StepHeader>
      <div className="rounded-lg bg-surface p-5">
        <output
          ref={stateRef}
          tabIndex={-1}
          aria-describedby={detail ? "setup-widevine-detail" : undefined}
          className="flex items-center gap-2 text-ui font-medium text-ink focus:outline-none"
        >
          {icon}
          <span>{state}</span>
        </output>
        {view === "downloading" && status && <DownloadBar status={status} />}
        {detail && (
          <p id="setup-widevine-detail" className="mt-2 max-w-[60ch] text-sub text-ink-muted">
            {detail}
            {retry && (
              <>
                {" "}
                <Trans>Sona also tries again by itself later.</Trans>
              </>
            )}
          </p>
        )}
        {retry && (
          <Button
            variant="outline"
            size="sm"
            className="mt-4"
            disabled={retrying}
            onClick={() => void onRetry()}
          >
            <RotateCw strokeWidth={1.75} />
            {retrying ? <Trans>Retrying…</Trans> : <Trans>Retry</Trans>}
          </Button>
        )}
      </div>
      <StepActions>
        <Button size="sm" onClick={onNext}>
          <Trans>Continue</Trans>
        </Button>
      </StepActions>
    </>
  );
}
