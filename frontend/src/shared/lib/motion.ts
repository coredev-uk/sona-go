import { createContext, useContext } from "react";

/**
 * Whether motion covers may play now: the motion artwork setting is on,
 * effects are not reduced (Sona's setting or the system's reduced motion)
 * and the machine is not on battery. The motion module provides it.
 */
export const MotionContext = createContext(false);

export function useMotionAllowed(): boolean {
  return useContext(MotionContext);
}

/**
 * A Go motion cover URL (`/motion/<key>/<size>.mp4`) for a cover drawn
 * `cssSize` CSS pixels wide, at twice that in device pixels; Go picks the
 * nearest rendition.
 */
export function motionAt(url: string, cssSize: number): string {
  const px = Math.min(2048, Math.max(16, Math.round(cssSize * 2)));
  return url.replace(/\/\d+\.mp4$/, `/${px}.mp4`);
}
