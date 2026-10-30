import { PlaybackService } from "@bindings/services";
import { Link } from "@tanstack/react-router";
import {
  CircleAlert,
  ListMusic,
  LoaderCircle,
  MicVocal,
  Pause,
  Play,
  Repeat,
  Repeat1,
  Shuffle,
  SkipBack,
  SkipForward,
  Volume1,
  Volume2,
  VolumeX,
} from "lucide-react";
import { useRef, useState, type ReactNode } from "react";

import { useOnAirProgramme } from "@/features/radio";
import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { Button } from "@/shared/ui/button";
import { Slider } from "@/shared/ui/slider";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { formatTime } from "@/shared/lib/format";
import { run } from "@/shared/lib/run";
import { cn } from "@/shared/lib/utils";

import { useLivePosition, usePlayback } from "./playback";
import { useUI } from "./ui";

function IconButton({
  label,
  disabled,
  pressed,
  onClick,
  children,
}: {
  label: string;
  disabled?: boolean;
  /** Toggle buttons (shuffle, repeat, queue, lyrics) report their state. */
  pressed?: boolean;
  onClick?: () => void;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          aria-pressed={pressed}
          disabled={disabled}
          onClick={onClick}
          className={cn(
            "relative text-ink-muted hover:text-ink disabled:opacity-40",
            // On: ink, with a dot under the glyph (amber is kept for "playing").
            pressed &&
              "text-ink after:absolute after:bottom-0.5 after:left-1/2 after:size-1 after:-translate-x-1/2 after:rounded-full after:bg-ink",
          )}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/** The seek bar: the only part of the bar that re-renders while playing. */
function Seek({ disabled }: { disabled: boolean }) {
  const position = useLivePosition();
  const durationMs = usePlayback((s) => s.durationMs);
  const [drag, setDrag] = useState<number | null>(null);
  const shown = drag ?? position;

  const commit = ([v]: number[]) => {
    setDrag(null);
    usePlayback.getState().patch({ positionMs: v, at: Date.now() });
    void run("Couldn't seek", PlaybackService.Seek(v));
  };

  return (
    <div
      className="flex w-full items-center gap-2.5"
      style={{ "--slider-range": "var(--amber)" } as React.CSSProperties}
    >
      <span className="w-10 text-right text-meta text-ink-faint num">
        {disabled ? "–:––" : formatTime(shown)}
      </span>
      <Slider
        aria-label="Seek"
        aria-valuetext={disabled ? undefined : `${formatTime(shown)} of ${formatTime(durationMs)}`}
        disabled={disabled || durationMs <= 0}
        value={[durationMs > 0 ? Math.min(shown, durationMs) : 0]}
        max={Math.max(durationMs, 1)}
        step={1000}
        onValueChange={([v]) => setDrag(v)}
        onValueCommit={commit}
      />
      <span className="w-10 text-meta text-ink-faint num">
        {disabled ? "–:––" : formatTime(durationMs)}
      </span>
    </div>
  );
}

/**
 * In place of the seek bar for live radio: a live marker and the
 * programme on air, from the station's schedule.
 */
function LiveLine({ id, playing }: { id: string; playing: boolean }) {
  const programme = useOnAirProgramme(id);
  const until = programme
    ? new Date(programme.endMs).toLocaleTimeString(undefined, {
        hour: "2-digit",
        minute: "2-digit",
      })
    : "";
  return (
    <div className="flex h-4 w-full min-w-0 items-center justify-center gap-2 text-meta">
      <span className="flex shrink-0 items-center gap-1.5 font-medium text-ink">
        <span
          aria-hidden
          className={cn("size-1.5 rounded-full", playing ? "bg-play" : "bg-ink-faint")}
        />
        Live
      </span>
      {programme && (
        <span className="min-w-0 truncate text-ink-muted">
          {programme.title}
          <span className="num"> · until {until}</span>
        </span>
      )}
    </div>
  );
}

function Volume() {
  const volume = usePlayback((s) => s.volume);
  const restore = useRef(0.8);
  const set = (v: number) => {
    usePlayback.getState().patch({ volume: v });
    void run("Couldn't change the volume", PlaybackService.SetVolume(v));
  };
  const VolumeIcon = volume === 0 ? VolumeX : volume < 0.5 ? Volume1 : Volume2;
  return (
    <div
      className="flex items-center gap-1"
      style={{ "--slider-range": "var(--ink-muted)" } as React.CSSProperties}
    >
      <IconButton
        label={volume === 0 ? "Unmute" : "Mute"}
        onClick={() => {
          if (volume > 0) restore.current = volume;
          set(volume > 0 ? 0 : restore.current || 0.8);
        }}
      >
        <VolumeIcon strokeWidth={1.75} />
      </IconButton>
      <Slider
        aria-label="Volume"
        className="w-24 shrink-0"
        value={[Math.round(volume * 100)]}
        max={100}
        step={1}
        onValueChange={([v]) => set(v / 100)}
      />
    </div>
  );
}

/**
 * The bottom bar: the cover and title, the transport with the seek bar,
 * and volume, queue and lyrics. It mirrors Go's playback:state; only the
 * seek bar ticks while playing.
 */
export function NowPlayingBar() {
  const current = usePlayback((s) => s.current);
  const state = usePlayback((s) => s.state);
  const shuffle = usePlayback((s) => s.shuffle);
  const repeat = usePlayback((s) => s.repeat);
  const canNext = usePlayback((s) => s.canNext);
  const canPrevious = usePlayback((s) => s.canPrevious);
  const error = usePlayback((s) => s.error);
  const authRequired = usePlayback((s) => s.authRequired);
  const { queueOpen, toggleQueue, overlay, openNowPlaying, closeOverlay, showLyrics } = useUI();

  const idle = !current;
  const playing = state === "playing";
  const loading = state === "loading";
  const lyricsOpen = overlay === "now-playing" && showLyrics;
  const nextRepeat = repeat === "off" ? "all" : repeat === "all" ? "one" : "off";
  const RepeatIcon = repeat === "one" ? Repeat1 : Repeat;

  let subline: ReactNode = null;
  if (authRequired) {
    subline = (
      <Link
        to="/settings"
        search={{ section: "account" }}
        className="truncate text-sub text-ink underline underline-offset-2"
      >
        Sign in again to keep playing
      </Link>
    );
  } else if (state === "error" && error) {
    subline = (
      // Go's error is engine detail; the line says what happened, the tooltip why.
      <span className="flex min-w-0 items-center gap-1.5 text-sub text-ink" title={error}>
        <CircleAlert aria-hidden className="size-3.5 shrink-0 text-danger" strokeWidth={2} />
        <span className="truncate">Couldn't play this song. Press play to try again.</span>
      </span>
    );
  } else if (current) {
    subline = (
      <span className="truncate text-sub text-ink-muted">
        {current.artistId ? (
          <Link
            to="/artist/$id"
            params={{ id: current.artistId }}
            className="hover:text-ink hover:underline hover:underline-offset-2"
          >
            {current.artistName}
          </Link>
        ) : (
          current.artistName
        )}
        {current.albumName && <> · {current.albumName}</>}
      </span>
    );
  }

  return (
    <section
      aria-label="Now playing"
      className="relative col-span-2 h-[72px] border-t border-line bg-surface"
    >
      <Backdrop src={current?.backdrop} />
      {/* The right column never shrinks below its controls (the volume
          slider collapsed to a dot in narrow windows); the transport gives
          way first, down to its minimum. */}
      <div className="relative grid h-full grid-cols-[minmax(0,1fr)_minmax(280px,560px)_minmax(max-content,1fr)] items-center gap-6 px-4">
        <div className="flex min-w-0 items-center gap-3">
          <button
            type="button"
            disabled={idle}
            onClick={() => (overlay === "now-playing" ? closeOverlay() : openNowPlaying(false))}
            aria-label={idle ? "Nothing playing" : "Open now playing"}
            className="shrink-0 rounded-sm"
          >
            <Artwork src={current?.art.small} size={48} alt="" eager />
          </button>
          <div className="flex min-w-0 flex-col">
            {current ? (
              current.live ? (
                <Link
                  to="/station/$id"
                  params={{ id: current.id }}
                  className="truncate text-ui font-medium text-ink hover:underline hover:underline-offset-2"
                >
                  {current.name}
                </Link>
              ) : current.albumId ? (
                <Link
                  to="/album/$id"
                  params={{ id: current.albumId }}
                  search={{ song: current.id }}
                  className="truncate text-ui font-medium text-ink hover:underline hover:underline-offset-2"
                >
                  {current.name}
                </Link>
              ) : (
                <span className="truncate text-ui font-medium text-ink">{current.name}</span>
              )
            ) : (
              <span className="truncate text-ui font-medium text-ink-muted">Not playing</span>
            )}
            {subline ?? (
              <span className="truncate text-sub text-ink-faint">
                Choose a song, album or playlist to start
              </span>
            )}
          </div>
          {current && !current.live && (current.catalogId || current.libraryId) && (
            <div className="flex shrink-0 items-center">
              <RatingButtons
                kind="songs"
                item={current}
                name={current.name}
                compact
                suggestLess={false}
              />
            </div>
          )}
        </div>

        <div className="flex w-full min-w-0 flex-col items-center gap-1">
          <div className="flex items-center gap-2">
            <IconButton
              label={shuffle ? "Shuffle on" : "Shuffle off"}
              pressed={shuffle}
              disabled={idle}
              onClick={() =>
                void run("Couldn't change shuffle", PlaybackService.SetShuffle(!shuffle))
              }
            >
              <Shuffle strokeWidth={1.75} />
            </IconButton>
            <IconButton
              label="Previous"
              disabled={idle || !canPrevious}
              onClick={() => void run("Couldn't go back", PlaybackService.Prev())}
            >
              <SkipBack strokeWidth={1.75} fill="currentColor" />
            </IconButton>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon"
                  aria-label={playing ? "Pause" : "Play"}
                  disabled={idle}
                  onClick={() => void run("Couldn't play or pause", PlaybackService.Toggle())}
                  className={cn(
                    "size-9 rounded-full bg-play text-on-play hover:bg-play/90",
                    "disabled:bg-hover disabled:text-ink-faint disabled:opacity-100",
                  )}
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
              <TooltipContent>
                {playing ? "Pause" : "Play"} <kbd className="ml-1 text-ink-faint">Space</kbd>
              </TooltipContent>
            </Tooltip>
            <IconButton
              label="Next"
              disabled={idle || !canNext}
              onClick={() => void run("Couldn't skip", PlaybackService.Next())}
            >
              <SkipForward strokeWidth={1.75} fill="currentColor" />
            </IconButton>
            <IconButton
              label={
                repeat === "off" ? "Repeat off" : repeat === "all" ? "Repeat all" : "Repeat one"
              }
              pressed={repeat !== "off"}
              disabled={idle}
              onClick={() =>
                void run("Couldn't change repeat", PlaybackService.SetRepeat(nextRepeat))
              }
            >
              <RepeatIcon strokeWidth={1.75} />
            </IconButton>
          </div>
          {current?.live ? (
            <LiveLine id={current.id} playing={playing} />
          ) : (
            <Seek disabled={idle} />
          )}
        </div>

        <div className="flex items-center justify-end gap-1">
          <IconButton
            label="Lyrics"
            pressed={lyricsOpen}
            disabled={idle}
            onClick={() => (lyricsOpen ? closeOverlay() : openNowPlaying(true))}
          >
            <MicVocal strokeWidth={1.75} />
          </IconButton>
          <IconButton label="Queue" pressed={queueOpen} onClick={toggleQueue}>
            <ListMusic strokeWidth={1.75} />
          </IconButton>
          <Volume />
        </div>
      </div>
    </section>
  );
}
