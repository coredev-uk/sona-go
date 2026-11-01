import { useEffect, useRef, useState } from "react";

import { Artwork } from "@/shared/components/Artwork";
import {
  motionAt,
  motionOutcome,
  motionStarted,
  motionStopped,
  useMotionState,
} from "@/shared/lib/motion";
import { cn } from "@/shared/lib/utils";

interface MotionCoverProps {
  /** Go's motion cover URL (Header.motion); "" or undefined for none. */
  motion?: string;
  /** The still cover (any Go artwork URL). */
  art?: string;
  /** Rendered edge length in CSS pixels. */
  size: number;
  /**
   * The size the video is requested for, when the cover changes size in
   * place (the now-playing view), so a resize does not reload it.
   */
  videoSize?: number;
  /** A round picture (artists). */
  shape?: "square" | "round";
  alt: string;
  eager?: boolean;
}

/**
 * A cover that moves: Apple's looping motion artwork over the still,
 * when there is one and motion is allowed (useMotionState). The still
 * shows until the first frame plays and stays if the video fails.
 */
export function MotionCover({ motion, art, size, videoSize, shape, alt, eager }: MotionCoverProps) {
  const { allowed, codec } = useMotionState();
  const src = allowed && motion ? motionAt(motion, videoSize ?? size, codec) : "";
  return (
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      <Artwork src={art} size={size} alt={alt} shape={shape} eager={eager} />
      {src && <MotionVideo key={src} src={src} round={shape === "round"} />}
    </div>
  );
}

/** How long a video may claim to play without its time moving. */
const STALL_MS = 4000;

/**
 * The muted loop over its parent (positioned) box. It plays only while on
 * screen, while the window is visible and while its page is not inert (under
 * the Now Playing view or full-screen lyrics, which cover the page, or
 * behind a modal dialog), and releases its decoder when it unmounts.
 *
 * The file goes to WebKit as a data: URL. WebKitGTK's media player refuses
 * the app's own scheme (wails://) outright, and from a blob: URL its
 * GStreamer source reads past the end of the file through byte ranges:
 * qtdemux then finds garbage ("atom has bogus size"), so about half the
 * loads failed or stalled and loops decoded corrupt frames. A data: URL
 * is decoded in one piece and plays every time (docs/frontend-architecture.md,
 * Motion artwork).
 */
export function MotionVideo({ src, round }: { src: string; round?: boolean }) {
  const ref = useRef<HTMLVideoElement>(null);
  const attempt = useRef(0);
  const [dataUrl, setDataUrl] = useState("");
  const [shown, setShown] = useState(false);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const abort = new AbortController();
    const id = motionStarted(src);
    attempt.current = id;
    fetch(src, { signal: abort.signal })
      .then(async (r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}: ${(await r.text()).trim()}`);
        const blob = await r.blob();
        return new Promise<string>((resolve, reject) => {
          const reader = new FileReader();
          reader.onload = () => resolve(String(reader.result));
          reader.onerror = () => reject(reader.error ?? new Error("FileReader failed"));
          reader.readAsDataURL(blob);
        });
      })
      .then((url) => {
        if (!abort.signal.aborted) setDataUrl(url);
      })
      .catch((err: unknown) => {
        if (abort.signal.aborted) return;
        setFailed(true);
        motionOutcome(id, "download-failed", err instanceof Error ? err.message : String(err));
      });
    return () => {
      abort.abort();
      motionStopped(id);
    };
  }, [src]);

  useEffect(() => {
    const video = ref.current;
    if (!video || !dataUrl) return;
    let onScreen = false;
    let stall = 0;
    // WebKit keeps decoding a video an overlay hides (IntersectionObserver
    // ignores occlusion), and the page under an overlay or a modal is
    // inert: the header under Now Playing made the web process use four
    // times what Now Playing uses alone (CONTRIBUTING.md, "Motion
    // artwork's cost").
    const sync = () => {
      if (onScreen && !video.closest("[inert]") && document.visibilityState === "visible")
        void video.play().catch(() => {});
      else video.pause();
    };
    const inert = new MutationObserver(sync);
    const io = new IntersectionObserver(([entry]) => {
      onScreen = entry.isIntersecting;
      sync();
    });
    const playing = () => {
      setShown(true);
      motionOutcome(attempt.current, "playing");
      const from = video.currentTime;
      window.clearTimeout(stall);
      stall = window.setTimeout(() => {
        if (!video.paused && video.currentTime === from)
          motionOutcome(
            attempt.current,
            "stalled",
            // eslint-disable-next-line lingui/no-unlocalized-strings -- a diagnostic detail for bug reports, kept in English like WebKit's own errors
            `WebKit reported playing but the video stayed at ${from.toFixed(2)} s for ${STALL_MS / 1000} s`,
          );
      }, STALL_MS);
    };
    const paused = () => motionStopped(attempt.current);
    const failure = () => {
      const e = video.error;
      setFailed(true);
      motionOutcome(
        attempt.current,
        "media-error",
        // eslint-disable-next-line lingui/no-unlocalized-strings -- WebKit's MediaError, verbatim for bug reports
        e ? `MediaError ${e.code}${e.message ? `: ${e.message}` : ""}` : "",
      );
    };
    io.observe(video);
    inert.observe(document.body, { attributes: true, attributeFilter: ["inert"], subtree: true });
    video.addEventListener("loadeddata", sync);
    video.addEventListener("playing", playing);
    video.addEventListener("pause", paused);
    video.addEventListener("error", failure);
    document.addEventListener("visibilitychange", sync);
    return () => {
      window.clearTimeout(stall);
      io.disconnect();
      inert.disconnect();
      video.removeEventListener("loadeddata", sync);
      video.removeEventListener("playing", playing);
      video.removeEventListener("pause", paused);
      video.removeEventListener("error", failure);
      document.removeEventListener("visibilitychange", sync);
      video.pause();
      // Drops WebKit's GStreamer pipeline now rather than at collection.
      video.removeAttribute("src");
      video.load();
    };
  }, [dataUrl]);

  if (failed || !dataUrl) return null;
  return (
    <video
      ref={ref}
      src={dataUrl}
      muted
      loop
      playsInline
      preload="auto"
      disablePictureInPicture
      aria-hidden
      tabIndex={-1}
      className={cn(
        // The same edge as Artwork's: an outline on the video itself.
        "pointer-events-none absolute inset-0 size-full object-cover outline -outline-offset-1 outline-ink/6 transition-opacity duration-300 ease-out",
        round ? "rounded-full" : "rounded-sm",
        shown ? "opacity-100" : "opacity-0",
      )}
    />
  );
}
