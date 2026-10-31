import { DesktopService, PlaybackService } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import {
  CircleAlert,
  LoaderCircle,
  Maximize2,
  Pause,
  Play,
  SkipBack,
  SkipForward,
  X,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useState, type CSSProperties } from "react";

import {
  activeLine,
  TrackAnnouncer,
  useLivePosition,
  useLyrics,
  usePlayback,
} from "@/features/player";
import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { callFailure } from "@/shared/lib/errors";
import { formatTime } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";
import { Slider } from "@/shared/ui/slider";

/** The window is dragged by its body; controls opt out. */
const drag = { "--wails-draggable": "drag" } as CSSProperties;
const noDrag = { "--wails-draggable": "no-drag" } as CSSProperties;

/** How long a failed command's message replaces the third line. */
const NOTICE_MS = 4000;

/** Tells failures apart, so a repeated one is a new live-region message. */
let noticeCount = 0;

/**
 * A command whose failure is said on the third line: the mini player has
 * no room for toasts.
 */
function useCommand() {
  const [notice, setNotice] = useState<{ text: string; id: number } | null>(null);
  const { t } = useLingui();
  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(null), NOTICE_MS);
    return () => window.clearTimeout(timer);
  }, [notice]);
  const command = (what: string, p: Promise<unknown>) =>
    void p.catch((err: unknown) => {
      const { code, message } = callFailure(err);
      const why =
        code === "not_signed_in" || code === "mut_rejected"
          ? t`sign in to Apple Music in Sona`
          : message;
      // A new object restarts the timer when the same failure repeats.
      setNotice({ text: `${what}: ${why}`, id: ++noticeCount });
    });
  return { notice, command };
}

function ChromeButton({
  label,
  icon: Icon,
  onClick,
}: {
  label: string;
  icon: LucideIcon;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      style={noDrag}
      className="grid size-6 place-items-center rounded-md text-ink-faint transition-colors duration-100 hover:bg-ink/8 hover:text-ink focus-visible:outline-2 focus-visible:outline-ring"
    >
      <Icon className="size-3.5" strokeWidth={2} />
    </button>
  );
}

function TransportButton({
  label,
  icon: Icon,
  disabled,
  onClick,
}: {
  label: string;
  icon: LucideIcon;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
      style={noDrag}
      className="grid size-7 place-items-center rounded-md text-ink-muted transition-colors duration-100 hover:bg-ink/8 hover:text-ink focus-visible:outline-2 focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-40"
    >
      <Icon className="size-4" strokeWidth={1.75} fill="currentColor" />
    </button>
  );
}

/** The seek bar with the time remaining; the only part that ticks. */
function Seek({ disabled, onSeek }: { disabled: boolean; onSeek: (ms: number) => void }) {
  const position = useLivePosition();
  const durationMs = usePlayback((s) => s.durationMs);
  const [drag, setDrag] = useState<number | null>(null);
  const shown = drag ?? position;
  const off = disabled || durationMs <= 0;
  const { t } = useLingui();
  const at = formatTime(shown);
  const total = formatTime(durationMs);
  return (
    <div
      className="flex min-w-0 flex-1 items-center gap-2"
      style={{ ...noDrag, "--slider-range": "var(--amber)" } as CSSProperties}
    >
      <Slider
        aria-label={t`Seek`}
        aria-valuetext={off ? undefined : t`${at} of ${total}`}
        disabled={off}
        value={[durationMs > 0 ? Math.min(shown, durationMs) : 0]}
        max={Math.max(durationMs, 1)}
        step={1000}
        onValueChange={([v]) => setDrag(v)}
        onValueCommit={([v]) => {
          setDrag(null);
          onSeek(v);
        }}
      />
      <span className="w-11 shrink-0 text-right text-meta text-ink-faint num">
        {off ? "–:––" : `−${formatTime(Math.max(durationMs - shown, 0))}`}
      </span>
    </div>
  );
}

/** The line being sung now, when Apple has synced lyrics for the song. */
function useLyricLine(catalogId: string): string {
  const lyrics = useLyrics(catalogId || undefined);
  const position = useLivePosition(250);
  const lines = lyrics.data?.lines ?? [];
  const synced = lyrics.data?.timing === "Line" || lyrics.data?.timing === "Word";
  if (!synced) return "";
  // Shown slightly early, like the lyrics view, so the eye arrives with the voice.
  const i = activeLine(lines, position + 250);
  return i >= 0 ? lines[i].text : "";
}

function ThirdLine({ catalogId, fallback }: { catalogId: string; fallback: string }) {
  const line = useLyricLine(catalogId);
  return (
    <p
      className={cn("truncate text-sub", line ? "text-ink" : "text-ink-faint")}
      aria-live="off"
      title={line || fallback}
    >
      {line || fallback}
    </p>
  );
}

/**
 * The mini player: a small always-on-top window with the cover, the song,
 * the line being sung, transport and seek. It mirrors Go's playback state
 * like the main window's bar; expanding returns to the main window.
 */
export function MiniPlayer() {
  const current = usePlayback((s) => s.current);
  const state = usePlayback((s) => s.state);
  const canNext = usePlayback((s) => s.canNext);
  const canPrevious = usePlayback((s) => s.canPrevious);
  const error = usePlayback((s) => s.error);
  const authRequired = usePlayback((s) => s.authRequired);
  const noCdm = usePlayback((s) => s.noCdm);
  const { notice, command } = useCommand();
  const { t } = useLingui();

  const idle = !current;
  const playing = state === "playing";
  const loading = state === "loading";

  let third = current?.albumName ?? "";
  if (authRequired) third = t`Sign in again in Sona to keep playing`;
  else if (noCdm) third = t`Waiting for Widevine: see Settings → Playback in Sona`;
  else if (state === "error" && error) third = t`Couldn't play this song. Press play to try again.`;
  else if (current?.live) third = t`Live radio`;

  return (
    <main
      aria-label={t`Sona mini player`}
      style={drag}
      className="relative flex h-full items-center gap-4 overflow-hidden bg-surface p-4 select-none"
    >
      <TrackAnnouncer />
      {/* Failures, said once each: a new paragraph per notice (WebKitGTK
          reports a live region's added children, not its changed text). */}
      <div role="status" className="sr-only">
        {notice && <p key={notice.id}>{notice.text}</p>}
      </div>
      <Backdrop src={current?.backdrop} />
      <div className="relative shrink-0">
        <Artwork src={current?.art.medium} size={112} alt="" eager />
      </div>

      <div className="relative flex h-28 min-w-0 flex-1 flex-col">
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-ui font-semibold text-ink" title={current?.name}>
              {current?.name ?? t`Not playing`}
            </h1>
            <p className="truncate text-sub text-ink-muted" title={current?.artistName}>
              {current ? current.artistName : t`Choose music in the main window`}
            </p>
          </div>
          <div className="-mt-0.5 -mr-1 flex shrink-0 items-center">
            <ChromeButton
              label={t`Open Sona`}
              icon={Maximize2}
              onClick={() => command(t`Couldn't open Sona`, DesktopService.ShowMainWindow())}
            />
            <ChromeButton
              label={t`Close mini player`}
              icon={X}
              onClick={() => command(t`Couldn't close`, DesktopService.CloseMiniPlayer())}
            />
          </div>
        </div>

        <div className="mt-1 min-h-[18px]">
          {notice ? (
            <p
              aria-hidden
              className="flex items-center gap-1.5 text-sub text-ink"
              title={notice.text}
            >
              <CircleAlert className="size-3.5 shrink-0 text-danger" strokeWidth={2} />
              <span className="truncate">{notice.text}</span>
            </p>
          ) : current && !current.live && !authRequired && !noCdm && state !== "error" ? (
            <ThirdLine catalogId={current.catalogId} fallback={third} />
          ) : (
            <p className="truncate text-sub text-ink-faint">{third}</p>
          )}
        </div>

        <div className="mt-auto flex items-center gap-3">
          <div className="flex shrink-0 items-center gap-1">
            <TransportButton
              label={t`Previous`}
              icon={SkipBack}
              disabled={idle || !canPrevious}
              onClick={() => command(t`Couldn't go back`, PlaybackService.Prev())}
            />
            <button
              type="button"
              aria-label={playing ? t`Pause` : t`Play`}
              title={playing ? t`Pause` : t`Play`}
              disabled={idle}
              onClick={() => command(t`Couldn't play or pause`, PlaybackService.Toggle())}
              style={noDrag}
              className="grid size-8 place-items-center rounded-full bg-play text-on-play transition-colors duration-100 hover:bg-play/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:bg-hover disabled:text-ink-faint"
            >
              {loading ? (
                <LoaderCircle className="size-4 animate-spin" strokeWidth={2} />
              ) : playing ? (
                <Pause className="size-4" fill="currentColor" strokeWidth={0} />
              ) : (
                <Play className="size-4 translate-x-px" fill="currentColor" strokeWidth={0} />
              )}
            </button>
            <TransportButton
              label={t`Next`}
              icon={SkipForward}
              disabled={idle || !canNext}
              onClick={() => command(t`Couldn't skip`, PlaybackService.Next())}
            />
          </div>
          {current?.live ? (
            <span className="flex items-center gap-1.5 text-meta font-medium text-ink">
              <span
                aria-hidden
                className={cn("size-1.5 rounded-full", playing ? "bg-play" : "bg-ink-faint")}
              />
              <Trans>Live</Trans>
            </span>
          ) : (
            <Seek
              disabled={idle}
              onSeek={(ms) => {
                usePlayback.getState().patch({ positionMs: ms, at: Date.now() });
                command(t`Couldn't seek`, PlaybackService.Seek(ms));
              }}
            />
          )}
        </div>
      </div>
    </main>
  );
}
