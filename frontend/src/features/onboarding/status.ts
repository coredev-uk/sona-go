import type { CDMStatus } from "@bindings/services";
import { t } from "@lingui/core/macro";

import { cdmFailedState, cdmPercent } from "@/features/cdm";

import type { WidevineView } from "./steps";

/** Widevine in a few words, for the step list and the summary. */
export function widevineShort(view: WidevineView, status: CDMStatus | null): string {
  switch (view) {
    case "loading":
      return "";
    case "unavailable":
    case "unsupported":
      return t`Not available here`;
    case "not-needed":
      return t`Not needed here`;
    case "waiting":
      return t`Starting…`;
    case "downloading": {
      const percent = status ? cdmPercent(status) : "";
      return percent ? t`Downloading, ${percent}` : t`Downloading…`;
    }
    case "ready": {
      const version = status?.version ?? "";
      return version ? t`Version ${version}` : t`Ready`;
    }
    case "failed":
      return status ? cdmFailedState(status.code) : "";
  }
}
