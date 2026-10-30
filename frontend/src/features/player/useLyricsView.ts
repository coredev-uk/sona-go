import type { LyricLine } from "@bindings/services";
import { useEffect, useRef, type RefObject } from "react";

import { useAppearance } from "@/features/settings";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";

import { activeLine, lyricsClock, useSongOffset } from "./lyrics";
import { useLivePosition } from "./playback";

/** After the user scrolls the lyrics, they are left alone this long. */
const USER_SCROLL_MS = 4_000;

/** The line sung now under the song's offset (-1 for none or unsynced lyrics). */
export function useSungLine(songId: string, lines: readonly LyricLine[], synced: boolean): number {
  const position = useLivePosition(200);
  const offset = useSongOffset(songId);
  return synced ? activeLine(lines, lyricsClock(position, offset)) : -1;
}

/** Reduce effects or the system's reduced motion: no smooth scrolling. */
export function useReducedMotion(): boolean {
  const reduceEffects = useAppearance((s) => s.reduceEffects);
  const system = useMediaQuery("(prefers-reduced-motion: reduce)");
  return reduceEffects || system;
}

/**
 * Keeps the sung line (the `[data-active]` element) in view inside the
 * scroll container `ref`, unless the user scrolled it in the last few
 * seconds. Returns the handler that records a user scroll (wheel, touch).
 */
export function useFollowLine(ref: RefObject<HTMLDivElement | null>, active: number) {
  const userScrolled = useRef(0);
  const reduced = useReducedMotion();

  useEffect(() => {
    const el = ref.current?.querySelector<HTMLElement>("[data-active]");
    if (!el || Date.now() - userScrolled.current < USER_SCROLL_MS) return;
    el.scrollIntoView({ block: "center", behavior: reduced ? "auto" : "smooth" });
  }, [ref, active, reduced]);

  return () => {
    userScrolled.current = Date.now();
  };
}
