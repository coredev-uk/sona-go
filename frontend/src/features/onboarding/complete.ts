import type { Settings } from "@bindings/config";
import { SystemService } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";

import { useUpdateSettings } from "@/features/settings";
import { keys } from "@/shared/lib/keys";
import { run } from "@/shared/lib/run";

/**
 * Marks the setup finished (config.Onboarding.Completed) through the
 * settings service, on the latest settings Go sent, so the main window no
 * longer opens on it. Already marked, nothing is saved. A failure is a
 * toast (useUpdateSettings', or the settings read's).
 */
export function useCompleteSetup(): () => Promise<void> {
  const qc = useQueryClient();
  const update = useUpdateSettings();
  return async () => {
    let s: Settings | undefined;
    const read = qc
      .ensureQueryData<Settings>({
        queryKey: keys.settings,
        queryFn: () => SystemService.Settings(),
      })
      .then((v) => (s = v));
    if (!(await run(t`Couldn't save the setting`, read)) || !s) return;
    if (!s.onboarding.completed) update.mutate({ ...s, onboarding: { completed: true } });
  };
}

/**
 * Leaves the setup for For you, marked finished. Focus goes to the page
 * (the shell's main, as the skip link does), never back to <body>.
 */
export function useLeaveSetup(): () => Promise<void> {
  const complete = useCompleteSetup();
  const navigate = useNavigate();
  return async () => {
    await complete();
    await navigate({ to: "/" });
    document.getElementById("content")?.focus();
  };
}
