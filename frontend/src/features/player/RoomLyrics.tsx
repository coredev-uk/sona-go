import type { LyricLine } from "@bindings/services";
import { Window } from "@wailsio/runtime";
import { Trans, useLingui } from "@lingui/react/macro";
import { Minimize2 } from "lucide-react";
import { memo, useEffect, useMemo, useRef, type KeyboardEvent, type ReactNode } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { callFailure } from "@/shared/lib/errors";
import { cn } from "@/shared/lib/utils";
import { useBinding } from "@/shared/shortcuts/bindings";
import { ChordKeys } from "@/shared/shortcuts/Keys";
import { Button } from "@/shared/ui/button";

import { isSynced, shownLine, useLyricsPrefs, type LyricsPrefs } from "./lyrics";
import { LyricBody, type LyricLangs } from "./LyricBody";
import { hasWordTiming, lineVoices, type Voice } from "./lyric-words";
import { LyricsTiming } from "./LyricsTiming";
import { usePlayback } from "./playback";
import { useLyrics } from "./queries";
import { useUI } from "./ui";
import {
  lineTone,
  useLyricLangs,
  useSungLine,
  useTurnPage,
  useVerses,
  type LineState,
} from "./useLyricsView";
import { useWordSync } from "./useWordSync";

/**
 * Room-sized type: about 40 characters across a 1920 px screen, so the
 * words can be read (and sung) from the sofa.
 */
const ROOM_TYPE =
  "text-[clamp(2.25rem,4.4vw,5.25rem)] leading-[1.1] font-bold tracking-[-0.02em] text-balance";

const RoomLine = memo(function RoomLine({
  index,
  line,
  state,
  prefs,
  langs,
  voice,
  together,
}: {
  index: number;
  line: LyricLine;
  state: LineState;
  prefs: LyricsPrefs;
  langs: LyricLangs;
  voice: Voice;
  together: boolean;
}) {
  return (
    <p
      dir="auto"
      data-line={index}
      data-active={state === "active" || undefined}
      aria-current={state === "active" ? "true" : undefined}
      className={cn(
        "py-[0.2em]",
        lineTone(state, line.words.length > 0),
        // A duet's other voice: indented, in italic.
        voice === "two" && "ps-[1.5em] italic",
      )}
    >
      {together && (
        <span className="mb-[0.1em] block text-[0.3em] leading-[1.3] font-medium tracking-normal text-ink-muted">
          <Trans>Together</Trans>
        </span>
      )}
      <LyricBody
        shown={shownLine(line, prefs)}
        langs={langs}
        subClassName={cn(
          "mt-[0.15em] text-[0.5em] leading-[1.2]",
          state === "past" ? "text-ink-faint" : "text-ink-muted",
        )}
      />
    </p>
  );
});

function RoomLines({ catalogId }: { catalogId: string }) {
  const lyrics = useLyrics(catalogId);
  const translation = useLyricsPrefs((s) => s.translation);
  const pronunciation = useLyricsPrefs((s) => s.pronunciation);
  const prefs = useMemo(() => ({ translation, pronunciation }), [translation, pronunciation]);
  const lines = useMemo(() => lyrics.data?.lines ?? [], [lyrics.data]);
  const synced = isSynced(lyrics.data);
  const { active, anchor } = useSungLine(catalogId, lines, synced);
  const scrollRef = useRef<HTMLDivElement>(null);
  const userScrolled = useTurnPage(scrollRef, anchor);
  useWordSync(scrollRef, catalogId, lines, hasWordTiming(lyrics.data), prefs);
  const voices = useMemo(() => lineVoices(lines), [lines]);
  const duet = voices.includes("two");
  const verses = useVerses(lines, synced);
  const langs = useLyricLangs(lyrics.data);
  const { t } = useLingui();

  if (lyrics.isPending) return <div aria-busy aria-label={t`Loading lyrics`} className="flex-1" />;
  if (lyrics.error && callFailure(lyrics.error).code !== "not_found") {
    return (
      <div className="relative flex min-h-0 flex-1 items-center justify-center px-[7vw]">
        <ErrorBlock
          className="w-full max-w-xl"
          title={t`Couldn't load the lyrics`}
          error={lyrics.error}
          onRetry={() => void lyrics.refetch()}
          retrying={lyrics.isRefetching}
        />
      </div>
    );
  }
  if (!lines.length)
    return (
      <RoomMessage>
        <Trans>No lyrics for this song.</Trans>
      </RoomMessage>
    );

  return (
    <div
      ref={scrollRef}
      onWheel={userScrolled}
      onTouchMove={userScrolled}
      onKeyDown={userScrolled}
      // Focusable so the arrow keys can scroll the sheet.
      tabIndex={0}
      role="group"
      aria-label={t`Lyrics`}
      className={cn(
        "relative no-scrollbar min-h-0 flex-1 overflow-y-auto lyrics-fade px-[7vw] pt-[12vh] pb-[70vh] [--fade:12vh] focus-visible:outline-offset-[-6px]",
        ROOM_TYPE,
      )}
    >
      {/* One sheet down the middle of the screen, read from one edge. */}
      <div className="mx-auto max-w-[24ch]">
        {verses.map((verse, v) => (
          <div key={v} data-verse className={cn(v > 0 && "mt-[0.9em]")}>
            {verse.map((i) => (
              <RoomLine
                key={i}
                index={i}
                line={lines[i]}
                prefs={prefs}
                langs={langs}
                voice={voices[i]}
                together={duet && voices[i] === "all" && voices[i - 1] !== "all"}
                state={
                  !synced
                    ? "plain"
                    : i === active
                      ? "active"
                      : i < Math.max(active, anchor)
                        ? "past"
                        : "future"
                }
              />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

function RoomMessage({ children }: { children: ReactNode }) {
  return (
    <div className="relative flex min-h-0 flex-1 items-center justify-center px-[7vw]">
      <p className={cn(ROOM_TYPE, "max-w-[24ch] text-ink-muted")}>{children}</p>
    </div>
  );
}

/**
 * Puts the window in full screen while mounted, and back on unmount if it
 * was not full screen before.
 */
function useWindowFullscreen() {
  useEffect(() => {
    let cancelled = false;
    let ours = false;
    void Window.IsFullscreen()
      .then((full) => {
        if (cancelled || full) return;
        ours = true;
        return Window.Fullscreen();
      })
      .catch(() => {});
    return () => {
      cancelled = true;
      if (ours) void Window.UnFullscreen().catch(() => {});
    };
  }, []);
}

/** Keeps Tab inside the view: the rest of the window is underneath it. */
function trapTab(e: KeyboardEvent<HTMLElement>) {
  if (e.key !== "Tab") return;
  const focusable = [
    ...e.currentTarget.querySelectorAll<HTMLElement>(
      "button:not([disabled]), a[href], [tabindex='0']",
    ),
  ];
  if (!focusable.length) return;
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault();
    first.focus();
  }
}

/**
 * Full-screen lyrics: the whole window (and the screen) given to the
 * words at room size, the sung line in amber, the song and the timing
 * nudge in a quiet strip at the foot. Escape returns to Now Playing.
 * Scrolling follows the song smoothly, or jumps under reduced motion.
 */
export function RoomLyrics() {
  const current = usePlayback((s) => s.current);
  const closeOverlay = useUI((s) => s.closeOverlay);
  const exitRef = useRef<HTMLButtonElement>(null);
  const exitKeys = useBinding("player.close-overlay");
  const { t } = useLingui();
  useWindowFullscreen();

  useEffect(() => exitRef.current?.focus(), []);
  // Nothing to read once the queue is cleared or the song has no lyrics.
  useEffect(() => {
    if (!current?.catalogId) closeOverlay();
  }, [current, closeOverlay]);
  if (!current?.catalogId) return null;

  return (
    <section
      aria-label={t`Lyrics, full screen`}
      onKeyDown={trapTab}
      className="fixed inset-0 z-50 flex animate-in flex-col bg-ground duration-200 fade-in"
    >
      <Backdrop src={current.backdrop} tone="ground" />
      <RoomLines key={current.catalogId} catalogId={current.catalogId} />
      <footer className="relative flex shrink-0 items-center gap-4 px-[7vw] pt-3 pb-7">
        <Artwork src={current.art.small} size={44} alt="" eager />
        <div className="min-w-0 flex-1">
          <p className="truncate text-ui font-medium text-ink">{current.name}</p>
          <p className="truncate text-sub text-ink-muted">{current.artistName}</p>
        </div>
        <LyricsTiming songId={current.catalogId} />
        <Button ref={exitRef} variant="ghost" size="sm" onClick={closeOverlay}>
          <Minimize2 strokeWidth={1.75} />
          <Trans>Exit full screen</Trans>
          {exitKeys[0] && <ChordKeys chord={exitKeys[0]} className="ml-1" />}
        </Button>
      </footer>
    </section>
  );
}
