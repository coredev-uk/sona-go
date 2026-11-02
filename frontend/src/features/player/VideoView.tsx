import { CatalogService, PlaybackService, type QueueItem } from "@bindings/services";
import { ErrorCode } from "@bindings/services/models";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Window } from "@wailsio/runtime";
import {
  CircleAlert,
  ExternalLink,
  LoaderCircle,
  Maximize,
  Minimize,
  Pause,
  Play,
  SkipBack,
  SkipForward,
  X,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type RefObject,
} from "react";

import { SDMark } from "@/shared/components/Badges";
import { artUrl } from "@/shared/lib/art";
import { useCapabilities } from "@/shared/lib/capabilities";
import { codeMessage } from "@/shared/lib/errors";
import { formatTime } from "@/shared/lib/format";
import { keys } from "@/shared/lib/keys";
import { run } from "@/shared/lib/run";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Slider } from "@/shared/ui/slider";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { seekBy, toggleMute } from "./commands";
import { IconButton, Volume } from "./controls";
import { useLivePosition, usePlayback } from "./playback";
import { trapTab } from "./trap-tab";
import { useUI } from "./ui";
import { fitPicture, runFrameLoop } from "./video-frames";
import { createRenderer } from "./video-render";

/** Left and Right in the view: a shorter step than the player's Shift+arrows. */
const SEEK_STEP_MS = 5_000;
/** The controls fade after this long without the pointer or a key, while playing. */
const IDLE_MS = 3_000;
/** The poster's request: 16:9, the picture box's usual size, so resizing never refetches it. */
const POSTER_BOX = { width: 960, height: 540 };
/** Before the first picture the box takes the video frame's shape. */
const VIDEO_SHAPE = { width: 16, height: 9 };
/** The view's keyboard help, read with the view's name. */
const KEYS_HELP = "video-view-keys";

type Size = { width: number; height: number };

/**
 * The window's full screen, toggled from the view, and left again when the
 * view closes if the view entered it. Follows changes the window manager
 * makes (F11, a double click on the title bar).
 */
function useFullscreen() {
  const [full, setFull] = useState(false);
  const ours = useRef(false);
  useEffect(() => {
    let live = true;
    const check = () =>
      void Window.IsFullscreen().then(
        (f) => live && setFull(f),
        () => {},
      );
    check();
    window.addEventListener("resize", check);
    return () => {
      live = false;
      window.removeEventListener("resize", check);
      if (ours.current) void Window.UnFullscreen().catch(() => {});
    };
  }, []);
  const set = useCallback((on: boolean) => {
    ours.current = on;
    setFull(on);
    void (on ? Window.Fullscreen() : Window.UnFullscreen()).catch(() => {});
  }, []);
  return [full, set] as const;
}

/** True once the pointer and the keys have been still for IDLE_MS; `wake` restarts the wait. */
function useIdle() {
  const [idle, setIdle] = useState(false);
  const timer = useRef(0);
  const wake = useCallback(() => {
    setIdle(false);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setIdle(true), IDLE_MS);
  }, []);
  useEffect(() => {
    timer.current = window.setTimeout(() => setIdle(true), IDLE_MS);
    return () => window.clearTimeout(timer.current);
  }, []);
  return [idle, wake] as const;
}

/** The element's content box, kept up to date. */
function useBox(ref: RefObject<HTMLElement | null>): Size {
  const [box, setBox] = useState<Size>({ width: 0, height: 0 });
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) =>
      setBox({ width: entry.contentRect.width, height: entry.contentRect.height }),
    );
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  return box;
}

/**
 * Draws Go's pictures into `canvas` while mounted (runFrameLoop): the
 * picture's size, the queue entry it belongs to, and whether pictures
 * have stopped coming while it waits. Each picture goes straight to the
 * canvas; React hears only of a new size, a new entry or a stall.
 */
function usePicture(canvas: RefObject<HTMLCanvasElement | null>) {
  const [picture, setPicture] = useState<(Size & { uid: number }) | null>(null);
  const [stalled, setStalled] = useState(false);
  useEffect(() => {
    const el = canvas.current;
    const renderer = el ? createRenderer(el) : null;
    if (!renderer) return;
    const abort = new AbortController();
    void runFrameLoop(abort.signal, {
      onPicture: (frame) => {
        renderer.draw(frame);
        const uid = usePlayback.getState().current?.uid ?? 0;
        setPicture((p) =>
          p && p.width === frame.width && p.height === frame.height && p.uid === uid
            ? p
            : { width: frame.width, height: frame.height, uid },
        );
        setStalled(false);
      },
      onWaiting: () => setStalled(true),
      onCleared: () => {
        renderer.clear();
        setPicture(null);
        setStalled(false);
      },
    });
    return () => {
      abort.abort();
      renderer.dispose();
    };
  }, [canvas]);
  return { picture, stalled };
}

/** The seek bar: elapsed, the slider and what is left, live from the playback mirror. */
function VideoSeek() {
  const position = useLivePosition();
  const durationMs = usePlayback((s) => s.durationMs);
  const [drag, setDrag] = useState<number | null>(null);
  const { t } = useLingui();
  const shown = durationMs > 0 ? Math.min(drag ?? position, durationMs) : 0;
  const at = formatTime(shown);
  const total = formatTime(durationMs);
  const left = formatTime(Math.max(durationMs - shown, 0));

  const commit = ([v]: number[]) => {
    setDrag(null);
    usePlayback.getState().patch({ positionMs: v, at: Date.now() });
    void run(t`Couldn't seek`, PlaybackService.Seek(v));
  };

  return (
    <div
      className="flex items-center gap-3"
      style={{ "--slider-range": "var(--amber)" } as React.CSSProperties}
    >
      <span className="w-12 shrink-0 text-right text-meta text-ink-muted num">{at}</span>
      <Slider
        aria-label={t`Seek`}
        aria-valuetext={t`${at} of ${total}`}
        disabled={durationMs <= 0}
        value={[shown]}
        max={Math.max(durationMs, 1)}
        step={1000}
        onValueChange={([v]) => setDrag(v)}
        onValueCommit={commit}
      />
      <span className="w-12 shrink-0 text-meta text-ink-muted num">
        <span aria-hidden>−{left}</span>
        <span className="sr-only">{t`${left} left`}</span>
      </span>
    </div>
  );
}

/** Opens the video on music.apple.com, once its page (with the link) has loaded. */
function WatchInAppleMusic({ id }: { id: string }) {
  const q = useQuery({
    queryKey: keys.catalogVideo(id),
    queryFn: () => CatalogService.MusicVideo(id),
    enabled: !!id,
  });
  const url = q.data?.video.url;
  const { t } = useLingui();
  if (!url) return null;
  return (
    <Button
      variant="outline"
      size="sm"
      onClick={() => void run(t`Couldn't open Apple Music`, CatalogService.OpenInAppleMusic(url))}
    >
      <ExternalLink strokeWidth={1.75} />
      <Trans>Watch in Apple Music</Trans>
    </Button>
  );
}

/** Why there is no picture, in the picture's place, with Apple Music as the way to watch. */
function VideoProblem({
  videoId,
  title,
  detail,
  onRetry,
}: {
  videoId: string;
  title: string;
  detail: string;
  onRetry?: () => void;
}) {
  return (
    <div
      role="alert"
      className="flex max-w-[520px] items-start gap-3 rounded-lg border border-dashed border-control-line px-5 py-4"
    >
      <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0 text-danger" strokeWidth={1.75} />
      <div className="min-w-0 flex-1">
        <p className="text-ui font-medium text-ink">{title}</p>
        <p className="mt-0.5 text-sub text-ink-muted">{detail}</p>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <WatchInAppleMusic id={videoId} />
          {onRetry && (
            <Button variant="ghost" size="sm" onClick={onRetry}>
              <Trans>Try again</Trans>
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * The playing music video over the whole window: its picture, letterboxed
 * on black at the picture's shape, between the title strip and the
 * transport. The controls fade while the video plays and nothing moves,
 * and come back with the pointer, any key, a pause, or focus.
 */
function VideoScreen({ current }: { current: QueueItem }) {
  const state = usePlayback((s) => s.state);
  const error = usePlayback((s) => s.error);
  const canNext = usePlayback((s) => s.canNext);
  const canPrevious = usePlayback((s) => s.canPrevious);
  const closeOverlay = useUI((s) => s.closeOverlay);
  const caps = useCapabilities();
  const [full, setFull] = useFullscreen();
  const [idle, wake] = useIdle();
  const [focusShown, setFocusShown] = useState(false);
  const rootRef = useRef<HTMLElement>(null);
  const stageRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const box = useBox(stageRef);
  const { picture, stalled } = usePicture(canvasRef);
  const { t } = useLingui();

  useEffect(() => rootRef.current?.focus(), []);

  const playing = state === "playing";
  const loading = state === "loading";
  const videoId = current.catalogId || current.id;
  // A picture of an earlier entry is not this video's.
  const shown = picture?.uid === current.uid ? picture : null;
  const volume = caps?.playback.volume ?? false;

  let problem: { title: string; detail: string; retry: boolean } | null = null;
  if (caps && !caps.playback.video) {
    problem = {
      title: t`Music videos don't play here`,
      detail: t`This platform or build of Sona can't play music videos. You can watch it in Apple Music.`,
      retry: false,
    };
  } else if (state === "error" && error) {
    problem = {
      title: t`Couldn't play this video`,
      detail: codeMessage(error),
      // Pressing play again can't change what Apple offers or refuses.
      retry: error !== ErrorCode.CodeVideoHDOnly && error !== ErrorCode.CodeVideoLicence,
    };
  }
  let status = "";
  if (!problem && (loading || playing)) {
    if (!shown) status = t`Loading the video…`;
    else if (loading || stalled) status = t`Buffering…`;
  }
  const controlsShown = !playing || !idle || focusShown || !!status || !!problem;
  const frame = fitPicture(shown ?? VIDEO_SHAPE, box);
  const toggle = () => void run(t`Couldn't play or pause`, PlaybackService.Toggle());

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    wake();
    trapTab(e);
    if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey) return;
    const target = e.target instanceof Element ? e.target : null;
    // Buttons keep Space, sliders their arrows; Shift+arrows are the player's 10 s seek.
    const onButton = !!target?.closest("button, a[href]");
    const onSlider = !!target?.closest("[role=slider]");
    const arrow = e.key === "ArrowLeft" || e.key === "ArrowRight";
    if (arrow && (onSlider || e.shiftKey)) return;
    switch (e.key) {
      case " ":
        if (onButton || onSlider) return;
        toggle();
        break;
      case "k":
      case "K":
        toggle();
        break;
      case "ArrowLeft":
        seekBy(-SEEK_STEP_MS);
        break;
      case "ArrowRight":
        seekBy(SEEK_STEP_MS);
        break;
      case "f":
      case "F":
        setFull(!full);
        break;
      case "m":
      case "M":
        if (!volume) return;
        toggleMute();
        break;
      case "Escape":
        if (full) setFull(false);
        else closeOverlay();
        break;
      default:
        return;
    }
    e.preventDefault();
  };
  // Shown while a control has keyboard focus, so Tab never lands on an unseen control.
  const onFocus = (e: FocusEvent<HTMLElement>) => {
    let visible = true;
    try {
      visible = e.target.matches(":focus-visible");
    } catch {
      // A web view without :focus-visible: count every focus.
    }
    setFocusShown(e.target !== e.currentTarget && visible);
  };
  const onBlur = (e: FocusEvent<HTMLElement>) => {
    if (!(e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget))) {
      setFocusShown(false);
    }
  };

  const fade = cn("transition-opacity duration-300", !controlsShown && "opacity-0");
  const artistName = current.artistName;

  return (
    <section
      ref={rootRef}
      tabIndex={-1}
      aria-label={t`Music video`}
      aria-describedby={KEYS_HELP}
      onKeyDown={onKeyDown}
      onPointerMove={wake}
      onPointerDown={wake}
      onFocus={onFocus}
      onBlur={onBlur}
      className={cn(
        "fixed inset-0 z-50 flex animate-in flex-col bg-ground duration-200 outline-none fade-in",
        !controlsShown && "cursor-none",
      )}
    >
      <p id={KEYS_HELP} className="sr-only">
        {volume ? (
          <Trans>
            Space or K plays and pauses, Left and Right go back or ahead 5 seconds, F switches full
            screen, M mutes, Escape closes the video.
          </Trans>
        ) : (
          <Trans>
            Space or K plays and pauses, Left and Right go back or ahead 5 seconds, F switches full
            screen, Escape closes the video.
          </Trans>
        )}
      </p>
      <header className={cn("relative flex h-16 shrink-0 items-center gap-4 px-8", fade)}>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <h2 className="truncate text-heading text-ink">
              <Link
                to="/music-video/$id"
                params={{ id: videoId }}
                onClick={closeOverlay}
                className="hover:underline hover:underline-offset-2"
              >
                {current.name}
              </Link>
            </h2>
            {!problem && <SDMark />}
            {/* A new paragraph per state: WebKitGTK announces added children only. */}
            <div role="status" className="flex shrink-0 items-center text-meta text-ink-muted">
              {status && (
                <p key={status} className="flex items-center gap-1.5">
                  <LoaderCircle aria-hidden className="size-3.5 animate-spin" strokeWidth={2} />
                  {status}
                </p>
              )}
            </div>
          </div>
          <p className="truncate text-sub text-ink-muted">
            {current.artistId ? (
              <Link
                to="/artist/$id"
                params={{ id: current.artistId }}
                onClick={closeOverlay}
                className="hover:text-ink hover:underline hover:underline-offset-2"
              >
                {artistName}
              </Link>
            ) : (
              artistName
            )}
          </p>
        </div>
        <IconButton
          label={full ? t`Exit full screen` : t`Full screen`}
          onClick={() => setFull(!full)}
        >
          {full ? <Minimize strokeWidth={1.75} /> : <Maximize strokeWidth={1.75} />}
        </IconButton>
        <IconButton label={t`Close video`} onClick={closeOverlay}>
          <X strokeWidth={1.75} />
        </IconButton>
      </header>

      <div ref={stageRef} className="relative mx-8 min-h-0 flex-1">
        <div className="absolute inset-0 grid place-items-center">
          {problem && (
            <VideoProblem
              videoId={videoId}
              title={problem.title}
              detail={problem.detail}
              onRetry={problem.retry ? toggle : undefined}
            />
          )}
          {/* Black behind the picture only: the box is the picture's own
              shape. It stays mounted under a problem, so its frame loop
              keeps running for a retry. */}
          <div
            className={cn("relative overflow-hidden bg-black", problem && "hidden")}
            style={{ width: frame.width, height: frame.height }}
          >
            <canvas
              ref={canvasRef}
              aria-hidden
              className={cn("absolute inset-0 size-full", !shown && "invisible")}
            />
            {!shown && current.art.large && (
              <img
                src={artUrl(current.art.large, POSTER_BOX)}
                alt=""
                decoding="async"
                draggable={false}
                className="absolute inset-0 size-full object-cover"
              />
            )}
          </div>
        </div>
      </div>

      <footer className={cn("relative shrink-0 px-8 pt-3 pb-6", fade)}>
        <VideoSeek />
        <div className="mt-2 grid grid-cols-[1fr_auto_1fr] items-center gap-4">
          <span />
          <div className="flex items-center gap-2">
            <IconButton
              label={t`Previous`}
              disabled={!canPrevious}
              onClick={() => void run(t`Couldn't go back`, PlaybackService.Prev())}
            >
              <SkipBack strokeWidth={1.75} fill="currentColor" />
            </IconButton>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon"
                  aria-label={playing ? t`Pause` : t`Play`}
                  onClick={toggle}
                  className="size-11 rounded-full bg-play text-on-play hover:bg-play/90"
                >
                  {loading ? (
                    <LoaderCircle className="animate-spin" strokeWidth={2} />
                  ) : playing ? (
                    <Pause fill="currentColor" strokeWidth={0} />
                  ) : (
                    <Play fill="currentColor" strokeWidth={0} className="translate-x-px" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{playing ? t`Pause` : t`Play`}</TooltipContent>
            </Tooltip>
            <IconButton
              label={t`Next`}
              disabled={!canNext}
              onClick={() => void run(t`Couldn't skip`, PlaybackService.Next())}
            >
              <SkipForward strokeWidth={1.75} fill="currentColor" />
            </IconButton>
          </div>
          <div className="flex items-center justify-end">
            <Volume />
          </div>
        </div>
      </footer>
    </section>
  );
}

/** How long the view waits for Go's state to name the video that opened it. */
const OPEN_WAIT_MS = 2_000;

/**
 * The video view (overlay "video"), for the music video playing. Keys:
 * Space or K play and pause, Left and Right seek 5 s, F full screen, M
 * mute, Escape leaves full screen, then closes the view; playback carries
 * on in the player bar, from which the view opens again. It closes when
 * the queue moves on from the video, and when the play that opened it
 * never reaches the playback mirror.
 */
export function VideoView() {
  const current = usePlayback((s) => s.current);
  const closeOverlay = useUI((s) => s.closeOverlay);
  const isVideo = !!current?.video;
  const seen = useRef(false);
  useEffect(() => {
    if (isVideo) {
      seen.current = true;
      return;
    }
    if (seen.current) {
      closeOverlay();
      return;
    }
    // Opened by playVideos before Go's state named the video: wait a moment.
    const id = window.setTimeout(closeOverlay, OPEN_WAIT_MS);
    return () => window.clearTimeout(id);
  }, [isVideo, closeOverlay]);
  return current?.video ? <VideoScreen current={current} /> : null;
}
