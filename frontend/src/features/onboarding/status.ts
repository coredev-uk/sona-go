import type { CDMStatus } from "@bindings/services";
import { t } from "@lingui/core/macro";

import { cdmFailedState, cdmPercent } from "@/features/cdm";

import type { WidevineView } from "./steps";

/** Widevine's state in a few words, for the welcome and the last step. */
export function widevineShort(view: WidevineView, status: CDMStatus | null): string {
  switch (view) {
    case "loading":
    case "not-needed":
      return "";
    case "unavailable":
    case "unsupported":
      return t`Not available here`;
    case "waiting":
      return t`Starting…`;
    case "downloading": {
      const percent = status ? cdmPercent(status) : "";
      return percent ? t`Downloading, ${percent}` : t`Downloading…`;
    }
    case "ready": {
      const version = status?.version ?? "";
      return version ? t`Ready, version ${version}` : t`Ready`;
    }
    case "failed":
      return status ? cdmFailedState(status.code) : "";
  }
}

/** Widevine's state as one item of the step list's line under Playback. */
export function widevineItem(view: WidevineView, status: CDMStatus | null): string {
  switch (view) {
    case "loading":
    case "not-needed":
      return "";
    case "unavailable":
    case "unsupported":
      return t`No playback here`;
    case "waiting":
      return t`Widevine starting…`;
    case "downloading": {
      const percent = status ? cdmPercent(status) : "";
      return percent ? t`Widevine ${percent}` : t`Widevine downloading…`;
    }
    case "ready":
      return t`Widevine ready`;
    case "failed":
      return t`Widevine failed`;
  }
}
