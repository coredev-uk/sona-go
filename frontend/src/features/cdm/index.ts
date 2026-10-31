import { CDMService } from "@bindings/services";

import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";

import { applyCdmStatus, useCdm } from "./state";

export { cdmProblem } from "./messages";
export { canRetryCdm, cdmPercent, cdmProgress, retryCdm, useCdm } from "./state";

/**
 * The Widevine CDM that Sona downloads from Google and keeps up to date by
 * itself: Go's CDMService mirrored from cdm:status. Settings → Playback
 * shows it with Retry, Diagnostics reads it beside the probe's checks, and
 * the player bar explains a song that waits for it (the snapshot's noCdm).
 */
export const cdmModule: FeatureModule<never> = {
  id: "cdm",
  routes: [],
  eventHandlers: {
    "cdm:status": (data, { invalidate }) => {
      // A new state or version changes what the playback probe finds; the
      // download's progress alone does not.
      if (applyCdmStatus(data)) invalidate([keys.diagnostics]);
    },
  },
  init: () => {
    // Go sends cdm:status for every change once the handler is bound, so a
    // status event that lands before this answer is at least as new: the
    // snapshot only fills the mirror while it is still empty (a download
    // that fails at once otherwise showed as idle).
    void CDMService.Status().then(
      (s) => {
        if (useCdm.getState().status === null) applyCdmStatus(s);
      },
      () => {},
    );
  },
};
