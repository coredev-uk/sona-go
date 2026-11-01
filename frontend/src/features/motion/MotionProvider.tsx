import { useMemo, useState, type ReactNode } from "react";

import { useAppearance, useSettings } from "@/features/settings";
import { MotionContext } from "@/shared/lib/motion";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";

import { H264_TYPE, HEVC_TYPE, motionState, usePower } from "./policy";

/** Gives covers (useMotionState) whether and how motion artwork plays now. */
export function MotionProvider({ children }: { children: ReactNode }) {
  const settings = useSettings();
  const reduceEffects = useAppearance((s) => s.reduceEffects);
  const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
  const power = usePower((s) => s.power);
  // WebKit answers from its GStreamer registry, fixed for the process.
  const [codecs] = useState(() => {
    const probe = document.createElement("video");
    return { h264: probe.canPlayType(H264_TYPE), hevc: probe.canPlayType(HEVC_TYPE) };
  });
  // Off until Go's settings arrive, so nothing starts and then stops.
  const setting = settings.data?.motionArtwork.enabled ?? false;
  const state = useMemo(
    () => motionState({ setting, reduceEffects, reducedMotion, power, ...codecs }),
    [setting, reduceEffects, reducedMotion, power, codecs],
  );
  return <MotionContext.Provider value={state}>{children}</MotionContext.Provider>;
}
