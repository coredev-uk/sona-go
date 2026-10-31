import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { CircleAlert } from "lucide-react";

import { cdmPercent, cdmProblem, useCdm } from "@/features/cdm";
import { cn } from "@/shared/lib/utils";

const linkClass = cn("truncate text-sub text-ink underline underline-offset-2");

/**
 * The player bar's line while the song waits for the Widevine CDM (the
 * snapshot's noCdm): still downloading, failed, or ready for another press
 * of play. It links to Settings → Playback, where the download's progress
 * and Retry are.
 */
export function CdmNotice() {
  const status = useCdm((s) => s.status);
  const { t } = useLingui();

  if (status?.state === "ready") {
    return (
      <span className="truncate text-sub text-ink">
        <Trans>Widevine is ready. Press play to start.</Trans>
      </span>
    );
  }
  if (status?.state === "failed") {
    return (
      <span
        className="flex min-w-0 items-center gap-1.5 text-sub text-ink"
        title={cdmProblem(status.code)}
      >
        <CircleAlert aria-hidden className="size-3.5 shrink-0 text-danger" strokeWidth={2} />
        <Link to="/settings" search={{ section: "playback" }} className={linkClass}>
          {status.code === "unsupported_platform"
            ? t`Songs can't play on this computer`
            : t`Couldn't get Widevine · Retry in Settings`}
        </Link>
      </span>
    );
  }
  const percent = status?.state === "downloading" ? cdmPercent(status) : "";
  return (
    <Link to="/settings" search={{ section: "playback" }} className={linkClass}>
      {percent
        ? t`Waiting for Widevine to download · ${percent}`
        : t`Waiting for Widevine to download`}
    </Link>
  );
}
