import { useEffect, useRef, useState } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { motionAt, useMotionAllowed } from "@/shared/lib/motion";
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
  alt: string;
  eager?: boolean;
}

/**
 * A cover that moves: Apple's looping motion artwork over the still,
 * when there is one and motion is allowed (useMotionAllowed). The still
 * shows until the first frame plays and stays if the video fails.
 */
export function MotionCover({ motion, art, size, videoSize, alt, eager }: MotionCoverProps) {
  const allowed = useMotionAllowed();
  const src = allowed && motion ? motionAt(motion, videoSize ?? size) : "";
  return (
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      <Artwork src={art} size={size} alt={alt} eager={eager} />
      {src && <MotionVideo key={src} src={src} />}
    </div>
  );
}

/**
 * The muted loop. It plays only while on screen and while the window is
 * visible, and releases its decoder when it unmounts. WebKitGTK's media
 * player refuses custom URL schemes (the app's wails://), so the file is
 * fetched and played from a blob: URL.
 */
function MotionVideo({ src }: { src: string }) {
  const ref = useRef<HTMLVideoElement>(null);
  const [blobUrl, setBlobUrl] = useState("");
  const [shown, setShown] = useState(false);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const abort = new AbortController();
    let url = "";
    fetch(src, { signal: abort.signal })
      .then((r) => (r.ok ? r.blob() : Promise.reject(new Error(String(r.status)))))
      .then((b) => {
        url = URL.createObjectURL(b);
        setBlobUrl(url);
      })
      .catch(() => {
        if (!abort.signal.aborted) setFailed(true);
      });
    return () => {
      abort.abort();
      if (url) URL.revokeObjectURL(url);
    };
  }, [src]);

  useEffect(() => {
    const video = ref.current;
    if (!video) return;
    let onScreen = false;
    const sync = () => {
      if (onScreen && document.visibilityState === "visible") void video.play().catch(() => {});
      else video.pause();
    };
    const io = new IntersectionObserver(([entry]) => {
      onScreen = entry.isIntersecting;
      sync();
    });
    io.observe(video);
    video.addEventListener("loadeddata", sync);
    document.addEventListener("visibilitychange", sync);
    return () => {
      io.disconnect();
      video.removeEventListener("loadeddata", sync);
      document.removeEventListener("visibilitychange", sync);
      video.pause();
      // Drops WebKit's GStreamer pipeline now rather than at collection.
      video.removeAttribute("src");
      video.load();
    };
  }, []);

  if (failed) return null;
  return (
    <>
      <video
        ref={ref}
        src={blobUrl || undefined}
        muted
        loop
        playsInline
        preload="auto"
        disablePictureInPicture
        aria-hidden
        tabIndex={-1}
        onPlaying={() => setShown(true)}
        onError={() => setFailed(true)}
        className={cn(
          "pointer-events-none absolute inset-0 size-full rounded-sm object-cover transition-opacity duration-300 ease-out",
          shown ? "opacity-100" : "opacity-0",
        )}
      />
      {shown && (
        <span
          aria-hidden
          className="pointer-events-none absolute inset-0 rounded-sm ring-1 ring-ink/6 ring-inset"
        />
      )}
    </>
  );
}
