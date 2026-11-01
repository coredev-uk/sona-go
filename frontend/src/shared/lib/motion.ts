import type { PowerState } from "@bindings/services";
import { createContext, useContext } from "react";
import { create } from "zustand";

/** The codec family motion renditions are asked for (Go's motion.Codec*). */
export type MotionCodec = "avc" | "hevc" | "";

/**
 * Everything that decides whether motion covers play, as the motion module
 * sees it now. `allowed` is the verdict; the rest is for Diagnostics.
 */
export interface MotionState {
  /** The motion module filled this in (false: no provider, e.g. in tests). */
  probed: boolean;
  allowed: boolean;
  /** Settings → Appearance → Motion artwork. */
  setting: boolean;
  /** Sona's Reduce effects. */
  reduceEffects: boolean;
  /** The system's prefers-reduced-motion. */
  reducedMotion: boolean;
  /** Go's power source (null until Go answers). */
  power: PowerState | null;
  /** HTMLMediaElement.canPlayType for Apple's H.264 and HEVC renditions. */
  h264: string;
  hevc: string;
  /** The codec asked for: H.264 where WebKit plays it, else HEVC, else none. */
  codec: MotionCodec;
}

const OFF: MotionState = {
  probed: false,
  allowed: false,
  setting: false,
  reduceEffects: false,
  reducedMotion: false,
  power: null,
  h264: "",
  hevc: "",
  codec: "",
};

/** The motion module's state, given to shared covers (motion's Provider slot). */
export const MotionContext = createContext<MotionState>(OFF);

export function useMotionState(): MotionState {
  return useContext(MotionContext);
}

/**
 * Whether motion covers may play now: the setting is on, effects are not
 * reduced (Sona's setting or the system's), the computer is not on
 * battery, and WebKit can decode a codec Apple offers.
 */
export function useMotionAllowed(): boolean {
  return useContext(MotionContext).allowed;
}

/**
 * A Go motion cover URL (`/motion/<key>/<size>.mp4`) for a cover drawn
 * `cssSize` CSS pixels wide at the screen's density (at most 2, as
 * shared/lib/art.ts), in `codec` (Go picks the nearest rendition; HEVC
 * only when asked for).
 */
export function motionAt(url: string, cssSize: number, codec: MotionCodec = "avc"): string {
  const density = Math.min(2, Math.max(1, globalThis.devicePixelRatio || 1));
  const px = Math.min(2048, Math.max(16, Math.round(cssSize * density)));
  const sized = url.replace(/\/\d+\.mp4(\?.*)?$/, `/${px}.mp4`);
  return codec === "hevc" ? `${sized}?codec=hevc` : sized;
}

/** How a motion video went. */
export type MotionOutcome = "loading" | "playing" | "download-failed" | "media-error" | "stalled";

export interface MotionAttempt {
  id: number;
  /** The /motion/ URL. */
  src: string;
  /** Date.now() when it started. */
  at: number;
  outcome: MotionOutcome;
  /** Go's or WebKit's own words for a failure ("" otherwise). */
  detail: string;
}

interface MotionLog {
  /** The newest video's attempt (null: none since Sona started). */
  last: MotionAttempt | null;
  /** Ids of the videos playing now. */
  playing: number[];
}

/** Motion videos' outcomes, for Diagnostics. */
export const useMotionLog = create<MotionLog>(() => ({ last: null, playing: [] }));

let nextAttempt = 1;

/** Starts logging a video for src; returns its id. */
export function motionStarted(src: string): number {
  const id = nextAttempt++;
  useMotionLog.setState({ last: { id, src, at: Date.now(), outcome: "loading", detail: "" } });
  return id;
}

/** Records an outcome of attempt id (kept only while it is the newest). */
export function motionOutcome(id: number, outcome: MotionOutcome, detail = ""): void {
  useMotionLog.setState((s) => ({
    last: s.last?.id === id ? { ...s.last, outcome, detail } : s.last,
    playing:
      outcome === "playing"
        ? [...s.playing.filter((p) => p !== id), id]
        : s.playing.filter((p) => p !== id),
  }));
}

/** Attempt id stopped playing (paused, off screen, unmounted). */
export function motionStopped(id: number): void {
  useMotionLog.setState((s) => ({ playing: s.playing.filter((p) => p !== id) }));
}

/**
 * The cover tile whose motion plays: tiles mark themselves with
 * `data-cover-tile` and a `[data-tile-motion=<id>]` marker inside; one
 * delegated set of listeners picks the hovered (else focused) tile after a
 * short dwell, so only one tile video ever runs and a pointer sweeping
 * across a shelf starts none.
 */
export const useTileMotion = create<{ active: string | null }>(() => ({ active: null }));

/** How long a tile must stay hovered or focused before its video loads. */
export const TILE_DWELL_MS = 400;

let listening = false;

/** Installs the delegated tile listeners once for the document. */
export function listenForTileMotion(doc: Document = document): void {
  if (listening) return;
  listening = true;
  let timer = 0;
  let hovered: Element | null = null;
  let focused: Element | null = null;
  const tileOf = (t: EventTarget | null) =>
    t instanceof Element ? t.closest("[data-cover-tile]") : null;
  const update = () => {
    const tile = hovered ?? focused;
    const id = tile?.querySelector<HTMLElement>("[data-tile-motion]")?.dataset.tileMotion ?? null;
    if (id && useTileMotion.getState().active === id) return;
    window.clearTimeout(timer);
    useTileMotion.setState({ active: null });
    if (id) timer = window.setTimeout(() => useTileMotion.setState({ active: id }), TILE_DWELL_MS);
  };
  doc.addEventListener("pointerover", (e) => {
    const tile = tileOf(e.target);
    if (tile !== hovered) {
      hovered = tile;
      update();
    }
  });
  doc.addEventListener("pointerout", (e) => {
    if (e.relatedTarget === null && hovered) {
      hovered = null;
      update();
    }
  });
  doc.addEventListener("focusin", (e) => {
    focused = tileOf(e.target);
    update();
  });
  doc.addEventListener("focusout", (e) => {
    if (e.relatedTarget === null) {
      focused = null;
      update();
    }
  });
}
