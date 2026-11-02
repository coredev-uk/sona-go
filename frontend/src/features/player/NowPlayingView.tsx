import { PlaybackService, type LyricLine } from "@bindings/services";
import { Link } from "@tanstack/react-router";
import { Trans, useLingui } from "@lingui/react/macro";
import { Disc3, Languages, Maximize2, MicVocal, SpellCheck, X } from "lucide-react";
import { memo, useEffect, useMemo, useRef } from "react";

import { Backdrop } from "@/shared/components/Backdrop";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { MotionCover } from "@/shared/components/MotionCover";
import { callFailure } from "@/shared/lib/errors";
import { useMotionAllowed } from "@/shared/lib/motion";
import { run } from "@/shared/lib/run";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";
import { cn } from "@/shared/lib/utils";
import { useBinding } from "@/shared/shortcuts/bindings";
import { ChordKeys } from "@/shared/shortcuts/Keys";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import {
  isSynced,
  lyricsExtras,
  seekTarget,
  shownLine,
  useLyricsOffset,
  useLyricsPrefs,
  type LyricsPrefs,
} from "./lyrics";
import { LyricsTiming } from "./LyricsTiming";
import { usePlayback } from "./playback";
import { useAlbumMotion, useLyrics } from "./queries";
import { useUI } from "./ui";
import { useFollowLine, useSungLine } from "./useLyricsView";

const Line = memo(function Line({
  songId,
  line,
  state,
  synced,
  prefs,
}: {
  songId: string;
  line: LyricLine;
  state: "past" | "active" | "future";
  synced: boolean;
  prefs: LyricsPrefs;
}) {
  const shown = shownLine(line, prefs);
  const { t } = useLingui();
  const body = (
    <>
      <span className="block">{shown.text}</span>
      {shown.background && (
        <span className="mt-1 block text-heading font-medium opacity-80">{shown.background}</span>
      )}
      {shown.translation && (
        <span className="mt-1.5 block text-heading font-medium opacity-70">
          {shown.translation}
        </span>
      )}
    </>
  );
  const cls = cn(
    "block w-full rounded-md px-3 py-2 text-left text-page text-balance transition-colors duration-150",
    state === "active" ? "text-play-ink" : state === "past" ? "text-ink-muted" : "text-ink",
    line.agent === "v2" && "text-right",
  );
  if (!synced) return <p className={cls}>{body}</p>;
  return (
    <button
      type="button"
      data-active={state === "active" || undefined}
      aria-current={state === "active" ? "true" : undefined}
      onClick={() => {
        const offset = useLyricsOffset.getState().offsets[songId] ?? 0;
        void run(t`Couldn't seek`, PlaybackService.Seek(seekTarget(line.beginMs, offset)));
      }}
      className={cn(cls, "hover:bg-ink/5")}
    >
      {body}
    </button>
  );
});

function LyricsPane({ catalogId }: { catalogId: string }) {
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

  if (lyrics.isPending) {
    return (
      <div aria-busy className="flex flex-col gap-5 px-3 pt-24">
        {[72, 56, 64, 40, 60].map((w, i) => (
          <div key={i} className="h-6 rounded-sm bg-ink/8" style={{ width: `${w}%` }} />
        ))}
      </div>
    );
  }
  if (lyrics.error) {
    const { code } = callFailure(lyrics.error);
    if (code === "not_found") return <NoLyrics />;
    return (
      <ErrorBlock
        className="mt-24"
        title={t`Couldn't load the lyrics`}
        error={lyrics.error}
        onRetry={() => void lyrics.refetch()}
        retrying={lyrics.isRefetching}
      />
    );
  }
  if (!lines.length) return <NoLyrics />;
  const writers = lyrics.data?.songwriters?.join(", ");
  const sources: Record<string, string> = {
    apple: t`Apple Music`,
    musixmatch: t`Musixmatch`,
    lrclib: t`LRCLIB`,
    netease: t`NetEase Cloud Music`,
  };
  const source = sources[lyrics.data?.provider ?? ""] ?? "";

  return (
    <div
      ref={scrollRef}
      onWheel={userScrolled}
      onTouchMove={userScrolled}
      className="no-scrollbar h-full overflow-y-auto py-[35vh] pr-2"
      aria-label={t`Lyrics`}
    >
      {lines.map((line, i) => (
        <Line
          key={i}
          songId={catalogId}
          line={line}
          synced={synced}
          state={!synced || i > active ? "future" : i === active ? "active" : "past"}
          prefs={prefs}
        />
      ))}
      {!!writers && (
        <p className="mt-10 px-3 text-sub text-ink-muted">
          <Trans>Written by {writers}</Trans>
        </p>
      )}
      {!!source && (
        <p className={cn("px-3 text-sub text-ink-muted", writers ? "mt-1" : "mt-10")}>
          <Trans>Lyrics from {source}</Trans>
        </p>
      )}
    </div>
  );
}

function NoLyrics() {
  return (
    <div className="mt-24 rounded-lg border border-dashed border-control-line px-6 py-6">
      <p className="text-heading text-ink">
        <Trans>No lyrics for this song</Trans>
      </p>
      <p className="mt-1 text-ui text-ink-muted">
        <Trans>None of the lyrics sources in Settings has lyrics for it.</Trans>
      </p>
    </div>
  );
}

/** A language tag's name in the UI's language ("en-US" → "English"). */
function languageName(tag: string, locale: string): string {
  try {
    return new Intl.DisplayNames(locale, { type: "language" }).of(tag.split("-")[0]) ?? tag;
  } catch {
    return tag;
  }
}

/**
 * The translation and pronunciation toggles, shown only when Apple has
 * them for this song. The choice is kept for every song.
 */
function LyricsOptions({ catalogId }: { catalogId: string }) {
  const lyrics = useLyrics(catalogId);
  const prefs = useLyricsPrefs();
  const has = lyricsExtras(lyrics.data);
  const { t, i18n } = useLingui();
  const language = languageName(lyrics.data?.translationLanguage ?? "", i18n.locale);
  const TOGGLE = "aria-pressed:bg-active aria-pressed:text-ink";
  return (
    <>
      {has.translation && (
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={prefs.translation}
          title={t`Translation into ${language}`}
          onClick={() => prefs.set({ translation: !prefs.translation })}
          className={TOGGLE}
        >
          <Languages strokeWidth={1.75} />
          <Trans>Translation</Trans>
        </Button>
      )}
      {has.pronunciation && (
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={prefs.pronunciation}
          title={t`The words in Latin letters`}
          onClick={() => prefs.set({ pronunciation: !prefs.pronunciation })}
          className={TOGGLE}
        >
          <SpellCheck strokeWidth={1.75} />
          <Trans>Pronunciation</Trans>
        </Button>
      )}
      {(has.translation || has.pronunciation) && (
        <span aria-hidden className="mx-1 h-4 w-px bg-line" />
      )}
    </>
  );
}

/** Timing and the full-screen view, for synced lyrics. */
function SyncedControls({ catalogId }: { catalogId: string }) {
  const lyrics = useLyrics(catalogId);
  const openRoom = useUI((s) => s.openRoom);
  const roomKeys = useBinding("player.lyrics-room");
  if (!isSynced(lyrics.data) || !lyrics.data?.lines.length) return null;
  return (
    <>
      <LyricsTiming songId={catalogId} />
      <span aria-hidden className="mx-1 h-4 w-px bg-line" />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="sm" onClick={openRoom}>
            <Maximize2 strokeWidth={1.75} />
            <Trans>Full screen</Trans>
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <Trans>Lyrics across the whole screen, large enough to read across a room</Trans>
          {roomKeys[0] && <ChordKeys chord={roomKeys[0]} className="ml-1.5" />}
        </TooltipContent>
      </Tooltip>
      <span aria-hidden className="mx-1 h-4 w-px bg-line" />
    </>
  );
}

/** The playing song's cover, moving when its album has motion artwork. */
function NowPlayingCover({
  albumId,
  art,
  size,
  alt,
}: {
  albumId: string;
  art: string;
  size: number;
  alt: string;
}) {
  const allowed = useMotionAllowed();
  const motion = useAlbumMotion(albumId, allowed);
  return <MotionCover motion={motion.data} art={art} size={size} videoSize={420} alt={alt} eager />;
}

/**
 * The full now-playing view over the page: the cover on a wash of its
 * colour and, when asked for, the time-synced lyrics with the sung line in
 * amber. Escape or the close button returns to the page.
 */
export function NowPlayingView() {
  const current = usePlayback((s) => s.current);
  const { showLyrics, setShowLyrics, closeOverlay } = useUI();
  const narrow = useMediaQuery("(max-width: 1100px)");
  const closeRef = useRef<HTMLButtonElement>(null);
  const { t } = useLingui();

  useEffect(() => closeRef.current?.focus(), []);
  // Nothing to show once the queue is cleared.
  useEffect(() => {
    if (!current) closeOverlay();
  }, [current, closeOverlay]);
  if (!current) return null;

  const lyricsOn = showLyrics && !!current.catalogId;
  const albumName = current.albumName;
  const cover = lyricsOn ? (narrow ? 200 : 320) : narrow ? 280 : 420;

  return (
    <section
      aria-label={t`Now playing`}
      className="absolute inset-0 z-20 flex animate-in flex-col overflow-hidden bg-ground duration-150 fade-in"
    >
      <Backdrop src={current.backdrop} tone="ground" />
      <div className="relative flex h-12 shrink-0 items-center justify-end gap-1 px-4">
        {lyricsOn && <LyricsOptions catalogId={current.catalogId} />}
        {lyricsOn && <SyncedControls catalogId={current.catalogId} />}
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={lyricsOn}
          disabled={!current.catalogId}
          onClick={() => setShowLyrics(!showLyrics)}
        >
          {lyricsOn ? <Disc3 strokeWidth={1.75} /> : <MicVocal strokeWidth={1.75} />}
          {lyricsOn ? t`Cover only` : t`Lyrics`}
        </Button>
        <Button
          ref={closeRef}
          variant="ghost"
          size="icon-sm"
          aria-label={t`Close now playing (Escape)`}
          onClick={closeOverlay}
        >
          <X strokeWidth={1.75} />
        </Button>
      </div>
      <div
        className={cn(
          "relative mx-auto grid min-h-0 w-full max-w-[1200px] flex-1 grid-rows-[minmax(0,1fr)] gap-16 px-10 pb-8",
          lyricsOn ? "grid-cols-[auto_minmax(0,1fr)]" : "grid-cols-1 place-items-center",
        )}
      >
        <div className={cn("flex flex-col self-center", !lyricsOn && "items-center text-center")}>
          <NowPlayingCover
            albumId={current.albumId}
            art={current.art.large}
            size={cover}
            alt={t`${albumName} cover`}
          />
          <div className="mt-6 min-w-0" style={{ maxWidth: cover }}>
            <h2 className="line-clamp-2 text-page text-ink">{current.name}</h2>
            <p className="mt-1 truncate text-heading font-medium text-ink-muted">
              {current.artistId ? (
                <Link
                  to="/artist/$id"
                  params={{ id: current.artistId }}
                  onClick={closeOverlay}
                  className="hover:text-ink hover:underline hover:underline-offset-2"
                >
                  {current.artistName}
                </Link>
              ) : (
                current.artistName
              )}
            </p>
            {current.albumName && (
              <p className="mt-0.5 truncate text-sub text-ink-muted">
                {current.albumId ? (
                  <Link
                    to="/album/$id"
                    params={{ id: current.albumId }}
                    onClick={closeOverlay}
                    className="hover:text-ink hover:underline hover:underline-offset-2"
                  >
                    {current.albumName}
                  </Link>
                ) : (
                  current.albumName
                )}
              </p>
            )}
          </div>
        </div>
        {lyricsOn && (
          <div className="h-full min-h-0">
            <LyricsPane catalogId={current.catalogId} />
          </div>
        )}
      </div>
    </section>
  );
}
