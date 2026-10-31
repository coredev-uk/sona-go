import { PrivateService } from "@bindings/services";
import type { PrivateState } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { create } from "zustand";

import { run } from "@/shared/lib/run";

/**
 * Private listening as Go applies it: `source` is "setting" (the saved
 * setting; switching saves it), "development" (a development build starts
 * private; switching lasts this session only) or "environment"
 * (SONA_PRIVATE=1 keeps it on). null until Go answers.
 */
export const usePrivate = create<{ state: PrivateState | null }>(() => ({ state: null }));

export function applyPrivate(state: PrivateState) {
  usePrivate.setState({ state });
}

/** Whether the switch can be turned off (SONA_PRIVATE=1 forces it on). */
export function isForced(s: PrivateState | null): boolean {
  return s?.source === "environment";
}

/** Switches private listening; Go's private:changed updates the mirror. */
export function setPrivate(on: boolean): Promise<boolean> {
  return run(
    on ? t`Couldn't turn on private listening` : t`Couldn't turn off private listening`,
    PrivateService.Set(on).then(applyPrivate),
  );
}
