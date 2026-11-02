import type { LyricLine, Lyrics } from "@bindings/services";
import { useEffect, useMemo, useRef, type RefObject } from "react";

import { useAppearance } from "@/features/settings";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";

import type { LyricLangs } from "./LyricBody";
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

/** The languages of the lyrics and their localizations, for `lang`. */
export function useLyricLangs(l: Lyrics | undefined): LyricLangs {
  const lead = l?.language ?? "";
  const pronunciation = l?.transliterationLanguage ?? "";
  const translation = l?.translationLanguage ?? "";
  return useMemo(() => ({ lead, pronunciation, translation }), [lead, pronunciation, translation]);
}

/** Reduce effects or the system's reduced motion: no smooth scrolling, words lit whole. */
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
