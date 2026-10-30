import type { ReactNode } from "react";

import { useAppearance, useSettings } from "@/features/settings";
import { MotionContext } from "@/shared/lib/motion";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";

import { motionAllowed, usePower } from "./policy";

/** Tells covers (useMotionAllowed) whether motion artwork may play now. */
export function MotionProvider({ children }: { children: ReactNode }) {
  const settings = useSettings();
  const reduceEffects = useAppearance((s) => s.reduceEffects);
  const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
  const onBattery = usePower((s) => s.onBattery);
  const allowed = motionAllowed({
    // Off until Go's settings arrive, so nothing starts and then stops.
    enabled: settings.data?.motionArtwork.enabled ?? false,
    reduceEffects,
    reducedMotion,
    onBattery,
  });
  return <MotionContext.Provider value={allowed}>{children}</MotionContext.Provider>;
}
