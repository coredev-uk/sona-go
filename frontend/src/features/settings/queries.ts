import { t } from "@lingui/core/macro";
import type { Settings } from "@bindings/config";
import { SystemService } from "@bindings/services";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { callFailure } from "@/shared/lib/errors";
import { keys } from "@/shared/lib/keys";

import { useAppearance } from "./appearance";

/** Applies Go's settings to the appearance mirror (theme and reduce effects). */
export function loadAppearance(s: Settings) {
  const theme = s.theme === "light" || s.theme === "dark" ? s.theme : "system";
  useAppearance.getState().load({ theme, reduceEffects: s.reduceEffects });
}

export function useSettings() {
  return useQuery({ queryKey: keys.settings, queryFn: () => SystemService.Settings() });
}

/**
 * Saves a settings change in Go (each control saves itself). The cache and
 * the appearance mirror update at once; settings:changed confirms.
 */
export function useUpdateSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (next: Settings) => SystemService.UpdateSettings(next),
    onMutate: (next) => {
      const prev = qc.getQueryData<Settings>(keys.settings);
      qc.setQueryData(keys.settings, next);
      loadAppearance(next);
      return { prev };
    },
    onError: (err, _next, ctx) => {
      if (ctx?.prev) {
        qc.setQueryData(keys.settings, ctx.prev);
        loadAppearance(ctx.prev);
      }
      toast.error(t`Couldn't save the setting`, { description: callFailure(err).message });
    },
    onSuccess: (saved) => qc.setQueryData(keys.settings, saved),
  });
}
