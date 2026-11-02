import { DesktopService, PlaybackService } from "@bindings/services";
import { Link } from "@tanstack/react-router";
import { Trans, useLingui } from "@lingui/react/macro";
import {
  CircleAlert,
  ListMusic,
  LoaderCircle,
  MicVocal,
  Pause,
  Play,
  PictureInPicture2,
  Repeat,
  Repeat1,
  Shuffle,
  SkipBack,
  SkipForward,
  Volume1,
  Volume2,
  VolumeX,
} from "lucide-react";
import { useState, type ReactNode } from "react";

import { EqualizerButton } from "@/features/equalizer";
import { PrivateButton } from "@/features/private";
import { useOnAirProgramme } from "@/features/radio";
import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { PreviewMark } from "@/shared/components/Badges";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { Button } from "@/shared/ui/button";
import { Slider } from "@/shared/ui/slider";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { openAccountDialog } from "@/shared/lib/account";
import { useCapabilities } from "@/shared/lib/capabilities";
import { codeMessage } from "@/shared/lib/errors";
import { clockTime, formatTime } from "@/shared/lib/format";
import { run } from "@/shared/lib/run";
import { cn } from "@/shared/lib/utils";
import { useBinding } from "@/shared/shortcuts/bindings";
import { ChordKeys } from "@/shared/shortcuts/Keys";

import { CdmNotice } from "./CdmNotice";
import { setVolume, toggleMute } from "./commands";
import { useLivePosition, usePlayback } from "./playback";
import { SleepTimer } from "./SleepTimer";
import { TrackAnnouncer } from "./TrackAnnouncer";
import { QUEUE_TOGGLE_ID, useUI } from "./ui";

function IconButton({
  id,
  label,
  disabled,
  pressed,
  onClick,
  children,
}: {
  id?: string;
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
          id={id}
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
  const { t } = useLingui();
  const at = formatTime(shown);
  const total = formatTime(durationMs);

  const commit = ([v]: number[]) => {
    setDrag(null);
    usePlayback.getState().patch({ positionMs: v, at: Date.now() });
    void run(t`Couldn't seek`, PlaybackService.Seek(v));
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
        aria-label={t`Seek`}
        aria-valuetext={disabled ? undefined : t`${at} of ${total}`}
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
 * In place of the seek bar for live radio: a live marker and what is on
 * air, as a broadcaster's stream names it, else the programme from the
 * station's schedule.
 */
function LiveLine({ id, playing }: { id: string; playing: boolean }) {
  const programme = useOnAirProgramme(id);
  const onAir = usePlayback((s) => s.onAir);
  const until = programme ? clockTime(programme.endMs) : "";
  return (
    <div className="flex h-4 w-full min-w-0 items-center justify-center gap-2 text-meta">
      <span className="flex shrink-0 items-center gap-1.5 font-medium text-ink">
        <span
          aria-hidden
          className={cn("size-1.5 rounded-full", playing ? "bg-play" : "bg-ink-faint")}
        />
        <Trans>Live</Trans>
      </span>
      {onAir ? (
        <span className="min-w-0 truncate text-ink-muted">
          {onAir.artist ? `${onAir.artist} – ${onAir.title}` : onAir.title}
        </span>
      ) : (
        programme && (
          <span className="min-w-0 truncate text-ink-muted">
            {programme.title}
            <span className="num">
              {" · "}
              <Trans>until {until}</Trans>
            </span>
          </span>
        )
      )}
    </div>
  );
}

/** Mute and the volume slider, where Sona sets the playback volume. */
function Volume() {
  const volume = usePlayback((s) => s.volume);
  const supported = useCapabilities()?.playback.volume ?? false;
  const VolumeIcon = volume === 0 ? VolumeX : volume < 0.5 ? Volume1 : Volume2;
  const { t } = useLingui();
  if (!supported) return null;
  return (
    <div
      className="flex items-center gap-1"
      style={{ "--slider-range": "var(--ink-muted)" } as React.CSSProperties}
    >
      <IconButton label={volume === 0 ? t`Unmute` : t`Mute`} onClick={toggleMute}>
        <VolumeIcon strokeWidth={1.75} />
      </IconButton>
      <Slider
        aria-label={t`Volume`}
        className="w-24 shrink-0"
        value={[Math.round(volume * 100)]}
        max={100}
        step={1}
        onValueChange={([v]) => setVolume(v / 100)}
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
  const noCdm = usePlayback((s) => s.noCdm);
  const preview = usePlayback((s) => s.preview);
  const { queueOpen, toggleQueue, overlay, openNowPlaying, closeOverlay, showLyrics } = useUI();
  const toggleKeys = useBinding("player.toggle");
  const { t } = useLingui();

  const idle = !current;
  const playing = state === "playing";
  const loading = state === "loading";
  const lyricsOpen = overlay === "now-playing" && showLyrics;
  const nextRepeat = repeat === "off" ? "all" : repeat === "all" ? "one" : "off";
  const RepeatIcon = repeat === "one" ? Repeat1 : Repeat;

  let subline: ReactNode = null;
  if (authRequired) {
    subline = (
      <button
        type="button"
        onClick={openAccountDialog}
        className="truncate text-start text-sub text-ink underline underline-offset-2"
      >
        <Trans>Sign in again to keep playing</Trans>
      </button>
    );
  } else if (noCdm) {
    subline = <CdmNotice />;
  } else if (state === "error" && error) {
    subline = (
      // The line says what happened, the tooltip why (Go's error code, worded).
      <span
        className="flex min-w-0 items-center gap-1.5 text-sub text-ink"
        title={codeMessage(error)}
      >
        <CircleAlert aria-hidden className="size-3.5 shrink-0 text-danger" strokeWidth={2} />
        <span className="truncate">
          <Trans>Couldn't play this song. Press play to try again.</Trans>
        </span>
      </span>
    );
  } else if (current && !current.artistName && !current.albumName) {
    // A broadcaster names no provider: its name stands alone, centred.
    subline = false;
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
      // "Player", so the landmark differs from the Now Playing view's.
      aria-label={t`Player`}
      className="relative col-span-2 h-[72px] border-t border-line bg-surface"
    >
      <TrackAnnouncer />
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
            aria-label={idle ? t`Nothing playing` : t`Open now playing`}
            className="shrink-0 rounded-sm"
          >
            <Artwork src={current?.art.small} size={48} alt="" eager />
          </button>
          <div className="flex min-w-0 flex-col">
            <div className="flex min-w-0 items-center gap-1.5">
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
                <span className="truncate text-ui font-medium text-ink-muted">
                  <Trans>Not playing</Trans>
                </span>
              )}
              {/* Apple's 30-second clip instead of the song (SONA_PREVIEW=1). */}
              {current && preview && <PreviewMark />}
            </div>
            {subline ?? (
              <span className="truncate text-sub text-ink-faint">
                <Trans>Choose a song, album or playlist to start</Trans>
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
              label={shuffle ? t`Shuffle on` : t`Shuffle off`}
              pressed={shuffle}
              disabled={idle}
              onClick={() =>
                void run(t`Couldn't change shuffle`, PlaybackService.SetShuffle(!shuffle))
              }
            >
              <Shuffle strokeWidth={1.75} />
            </IconButton>
            <IconButton
              label={t`Previous`}
              disabled={idle || !canPrevious}
              onClick={() => void run(t`Couldn't go back`, PlaybackService.Prev())}
            >
              <SkipBack strokeWidth={1.75} fill="currentColor" />
            </IconButton>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon"
                  aria-label={playing ? t`Pause` : t`Play`}
                  disabled={idle}
                  onClick={() => void run(t`Couldn't play or pause`, PlaybackService.Toggle())}
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
                {playing ? t`Pause` : t`Play`}
                {toggleKeys[0] && <ChordKeys chord={toggleKeys[0]} className="ml-1.5" />}
              </TooltipContent>
            </Tooltip>
            <IconButton
              label={t`Next`}
              disabled={idle || !canNext}
              onClick={() => void run(t`Couldn't skip`, PlaybackService.Next())}
            >
              <SkipForward strokeWidth={1.75} fill="currentColor" />
            </IconButton>
            <IconButton
              label={
                repeat === "off" ? t`Repeat off` : repeat === "all" ? t`Repeat all` : t`Repeat one`
              }
              pressed={repeat !== "off"}
              disabled={idle}
              onClick={() =>
                void run(t`Couldn't change repeat`, PlaybackService.SetRepeat(nextRepeat))
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
          <PrivateButton />
          <SleepTimer disabled={idle} />
          <IconButton
            label={t`Lyrics`}
            pressed={lyricsOpen}
            disabled={idle}
            onClick={() => (lyricsOpen ? closeOverlay() : openNowPlaying(true))}
          >
            <MicVocal strokeWidth={1.75} />
          </IconButton>
          <IconButton
            id={QUEUE_TOGGLE_ID}
            label={t`Queue`}
            pressed={queueOpen}
            onClick={toggleQueue}
          >
            <ListMusic strokeWidth={1.75} />
          </IconButton>
          <IconButton
            label={t`Mini player`}
            onClick={() =>
              void run(t`Couldn't open the mini player`, DesktopService.ToggleMiniPlayer())
            }
          >
            <PictureInPicture2 strokeWidth={1.75} />
          </IconButton>
          <EqualizerButton />
          <Volume />
        </div>
      </div>
    </section>
  );
}
