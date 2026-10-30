import { DesktopService } from "@bindings/services";
import { useRouterState } from "@tanstack/react-router";
import { useEffect } from "react";

import { useSettings } from "@/features/settings";

/** Navigation settles before the route is saved: one write per page, not per keystroke. */
const SAVE_DELAY_MS = 1000;

/**
 * "Resume where you left off" (Settings → Desktop): saves the main
 * window's route in Go as it changes, debounced, while the setting is on.
 * Go opens the next start on it (config.Desktop.StartRoute).
 */
export function useRouteMemory() {
  const href = useRouterState({ select: (s) => s.location.href });
  const resume = useSettings().data?.desktop.resumeLastRoute ?? false;
  useEffect(() => {
    if (!resume) return;
    const t = window.setTimeout(() => {
      // Losing one route is harmless; the next navigation saves again.
      DesktopService.SetLastRoute(href).catch(() => {});
    }, SAVE_DELAY_MS);
    return () => window.clearTimeout(t);
  }, [href, resume]);
}
