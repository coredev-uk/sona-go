import type { LyricLine } from "@bindings/services";
import { Window } from "@wailsio/runtime";
import { Trans, useLingui } from "@lingui/react/macro";
import { Minimize2 } from "lucide-react";
import { memo, useEffect, useMemo, useRef, type KeyboardEvent, type ReactNode } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { callFailure } from "@/shared/lib/errors";
import { cn } from "@/shared/lib/utils";
import { useBinding } from "@/shared/shortcuts/bindings";
import { ChordKeys } from "@/shared/shortcuts/Keys";
import { Button } from "@/shared/ui/button";

import { isSynced, shownLine, useLyricsPrefs, type LyricsPrefs } from "./lyrics";
import { LyricsTiming } from "./LyricsTiming";
import { usePlayback } from "./playback";
import { useLyrics } from "./queries";
import { useUI } from "./ui";
import { useFollowLine, useSungLine } from "./useLyricsView";

/**
 * Room-sized type: about 40 characters across a 1920 px screen, so the
 * words can be read (and sung) from the sofa.
 */
const ROOM_TYPE =
  "text-[clamp(2.25rem,4.4vw,5.25rem)] leading-[1.1] font-bold tracking-[-0.02em] text-balance";

const RoomLine = memo(function RoomLine({
  line,
  state,
  prefs,
}: {
  line: LyricLine;
  state: "past" | "active" | "future" | "plain";
  prefs: LyricsPrefs;
}) {
  const shown = shownLine(line, prefs);
  return (
    <p
      data-active={state === "active" || undefined}
      aria-current={state === "active" ? "true" : undefined}
      className={cn(
        "max-w-[22ch] py-[0.22em] transition-colors duration-200 ease-out",
        state === "active"
          ? "text-play-ink"
          : state === "past"
            ? "text-ink-faint"
            : state === "plain"
              ? "text-ink"
              : "text-ink-muted",
        line.agent === "v2" && "ml-auto text-right",
      )}
    >
      {shown.text}
      {shown.background && (
        <span className="mt-[0.15em] block text-[0.5em] leading-[1.2] font-semibold">
          {shown.background}
        </span>
      )}
      {shown.translation && (
        <span className="mt-[0.2em] block text-[0.45em] leading-[1.25] font-medium">
          {shown.translation}
        </span>
      )}
    </p>
  );
});

function RoomLines({ catalogId }: { catalogId: string }) {
  const lyrics = useLyrics(catalogId);
  const translation = useLyricsPrefs((s) => s.translation);
  const pronunciation = useLyricsPrefs((s) => s.pronunciation);
  const prefs = useMemo(() => ({ translation, pronunciation }), [translation, pronunciation]);
  const lines = lyrics.data?.lines ?? [];
  const synced = isSynced(lyrics.data);
  const active = useSungLine(catalogId, lines, synced);
  const scrollRef = useRef<HTMLDivElement>(null);
  const userScrolled = useFollowLine(scrollRef, active);
  const { t } = useLingui();

  if (lyrics.isPending) return <div aria-busy aria-label={t`Loading lyrics`} className="flex-1" />;
  if (lyrics.error && callFailure(lyrics.error).code !== "not_found") {
    return (
      <RoomMessage>
        <Trans>Couldn't load the lyrics.</Trans>
      </RoomMessage>
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
      // Focusable so the arrow keys can scroll plain-text lyrics.
      tabIndex={0}
      aria-label={t`Lyrics`}
      className={cn(
        "relative no-scrollbar min-h-0 flex-1 overflow-y-auto px-[7vw] py-[38vh] focus-visible:outline-offset-[-6px]",
        ROOM_TYPE,
      )}
    >
      {lines.map((line, i) => (
        <RoomLine
          key={i}
          line={line}
          prefs={prefs}
          state={!synced ? "plain" : i > active ? "future" : i === active ? "active" : "past"}
        />
      ))}
    </div>
  );
}

function RoomMessage({ children }: { children: ReactNode }) {
  return (
    <div className="relative flex min-h-0 flex-1 items-center px-[7vw]">
      <p className={cn(ROOM_TYPE, "text-ink-muted")}>{children}</p>
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
