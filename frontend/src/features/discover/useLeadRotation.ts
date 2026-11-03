import { useCallback, useEffect, useState, type RefObject } from "react";

import { useAppearance } from "@/features/settings";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";

/** How long the lead stays before the next featured card replaces it. */
export const ROTATE_MS = 7000;

/**
 * Whether the lead is open to rotating at all: not under Sona's Reduce
 * effects nor the system's reduced motion, which keep the lead still and
 * leave the list as the way to change it.
 */
export function useRotationAllowed(): boolean {
  const reduceEffects = useAppearance((s) => s.reduceEffects);
  const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
  return !reduceEffects && !reducedMotion;
}

/** Whether `root` has the pointer over it, or focus inside it. */
function useEngaged(root: RefObject<HTMLElement | null>): boolean {
  const [hover, setHover] = useState(false);
  const [focus, setFocus] = useState(false);
  useEffect(() => {
    const el = root.current;
    if (!el) return;
    const enter = () => setHover(true);
    const leave = () => setHover(false);
    const focusin = () => setFocus(true);
    // Focus moving between two controls inside is not a leave.
    const focusout = (e: FocusEvent) =>
      setFocus(e.relatedTarget instanceof Node && el.contains(e.relatedTarget));
    el.addEventListener("pointerenter", enter);
    el.addEventListener("pointerleave", leave);
    el.addEventListener("focusin", focusin);
    el.addEventListener("focusout", focusout);
    return () => {
      el.removeEventListener("pointerenter", enter);
      el.removeEventListener("pointerleave", leave);
      el.removeEventListener("focusin", focusin);
      el.removeEventListener("focusout", focusout);
    };
  }, [root]);
  return hover || focus;
}

/**
 * Whether the user can see `root`: the window is visible (Page Visibility;
 * WebKitGTK reports a window on another workspace as hidden) and has focus,
 * and `root` is on screen.
 */
function useSeen(root: RefObject<HTMLElement | null>): boolean {
  const [visible, setVisible] = useState(() => document.visibilityState === "visible");
  const [focused, setFocused] = useState(() => document.hasFocus());
  const [onScreen, setOnScreen] = useState(true);
  useEffect(() => {
    const visibility = () => setVisible(document.visibilityState === "visible");
    const focus = () => setFocused(true);
    const blur = () => setFocused(false);
    document.addEventListener("visibilitychange", visibility);
    window.addEventListener("focus", focus);
    window.addEventListener("blur", blur);
    return () => {
      document.removeEventListener("visibilitychange", visibility);
      window.removeEventListener("focus", focus);
      window.removeEventListener("blur", blur);
    };
  }, []);
  useEffect(() => {
    const el = root.current;
    if (!el || typeof IntersectionObserver === "undefined") return;
    const io = new IntersectionObserver(([entry]) => setOnScreen(entry.isIntersecting));
    io.observe(el);
    return () => io.disconnect();
  }, [root]);
  return visible && focused && onScreen;
}

/**
 * What only the document can say when a tick comes: a menu is open (its
 * focus sits in a portal, outside `root`), the page is inert (under Now
 * Playing, lyrics or a modal), or a video plays in `root` (motion artwork
 * is left to play).
 */
function occupied(root: HTMLElement | null): boolean {
  if (document.querySelector('[role="menu"]')) return true;
  if (!root) return false;
  if (root.closest("[inert]")) return true;
  return [...root.querySelectorAll("video")].some((v) => !v.paused && !v.ended);
}

export interface LeadRotation {
  /** Rotation is on offer (not reduced motion, and more than one card). */
  available: boolean;
  /** The user's pause (the control's state), not the temporary holds. */
  paused: boolean;
  toggle: () => void;
}

/**
 * Advances the lead every ROTATE_MS: one timer, no per-frame work. It runs
 * only while `available`, not paused by the user, with no pointer or focus
 * inside `root`, the window visible and focused, and `root` on screen; a
 * tick that finds a menu open, the page inert or a video playing waits a
 * whole interval more. Each change of `at` (the timer's or the user's)
 * restarts the interval, so a chosen card is read for as long as any.
 * `advance` is stable while `at` is: it is a dependency of the timer.
 */
export function useLeadRotation(
  root: RefObject<HTMLElement | null>,
  { count, at, advance }: { count: number; at: number; advance: () => void },
): LeadRotation {
  const allowed = useRotationAllowed();
  const [paused, setPaused] = useState(false);
  const engaged = useEngaged(root);
  const seen = useSeen(root);
  const available = allowed && count > 1;
  const running = available && !paused && !engaged && seen;

  useEffect(() => {
    if (!running) return;
    let id = 0;
    const arm = () => {
      id = window.setTimeout(() => {
        if (occupied(root.current)) arm();
        else advance();
      }, ROTATE_MS);
    };
    arm();
    return () => window.clearTimeout(id);
  }, [running, at, advance, root]);

  const toggle = useCallback(() => setPaused((p) => !p), []);
  return { available, paused, toggle };
}
