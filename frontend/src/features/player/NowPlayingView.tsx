import { PlaybackService, type LyricLine } from "@bindings/services";
import { Link } from "@tanstack/react-router";
import { Disc3, Languages, MicVocal, SpellCheck, X } from "lucide-react";
import { memo, useEffect, useMemo, useRef } from "react";

import { useAppearance } from "@/features/settings";
import { Artwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { callFailure } from "@/shared/lib/errors";
import { run } from "@/shared/lib/run";
import { useMediaQuery } from "@/shared/lib/useMediaQuery";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";

import { activeLine, lyricsExtras, shownLine, useLyricsPrefs, type LyricsPrefs } from "./lyrics";
import { useLivePosition, usePlayback } from "./playback";
import { useLyrics } from "./queries";
import { useUI } from "./ui";

const Line = memo(function Line({
  line,
  state,
  synced,
  prefs,
}: {
  line: LyricLine;
  state: "past" | "active" | "future";
  synced: boolean;
  prefs: LyricsPrefs;
}) {
  const shown = shownLine(line, prefs);
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
      onClick={() => void run("Couldn't seek", PlaybackService.Seek(line.beginMs))}
      className={cn(cls, "hover:bg-ink/5")}
    >
      {body}
    </button>
  );
});

function LyricsPane({ catalogId }: { catalogId: string }) {
  const lyrics = useLyrics(catalogId);
  const position = useLivePosition(200);
  const reduce = useAppearance((s) => s.reduceEffects);
  const scrollRef = useRef<HTMLDivElement>(null);
  const userScrolled = useRef(0);
  const translation = useLyricsPrefs((s) => s.translation);
  const pronunciation = useLyricsPrefs((s) => s.pronunciation);
  const prefs = useMemo(() => ({ translation, pronunciation }), [translation, pronunciation]);

  const lines = lyrics.data?.lines ?? [];
  const synced = lyrics.data?.timing === "Line" || lyrics.data?.timing === "Word";
  // Lines are shown slightly early so the eye arrives as the voice does.
  const active = synced ? activeLine(lines, position + 250) : -1;

  useEffect(() => {
    const el = scrollRef.current?.querySelector<HTMLElement>("[data-active]");
    if (!el || Date.now() - userScrolled.current < 4000) return;
    el.scrollIntoView({ block: "center", behavior: reduce ? "auto" : "smooth" });
  }, [active, reduce]);

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
        title="Couldn't load the lyrics"
        error={lyrics.error}
        onRetry={() => void lyrics.refetch()}
        retrying={lyrics.isRefetching}
      />
    );
  }
  if (!lines.length) return <NoLyrics />;

  return (
    <div
      ref={scrollRef}
      onWheel={() => (userScrolled.current = Date.now())}
      onTouchMove={() => (userScrolled.current = Date.now())}
      className="no-scrollbar h-full overflow-y-auto py-[35vh] pr-2"
      aria-label="Lyrics"
    >
      {lines.map((line, i) => (
        <Line
          key={i}
          line={line}
          synced={synced}
          state={!synced || i > active ? "future" : i === active ? "active" : "past"}
          prefs={prefs}
        />
      ))}
      {!!lyrics.data?.songwriters?.length && (
        <p className="mt-10 px-3 text-sub text-ink-muted">
          Written by {lyrics.data.songwriters.join(", ")}
        </p>
      )}
    </div>
  );
}

function NoLyrics() {
  return (
    <div className="mt-24 rounded-lg border border-dashed border-control-line px-6 py-6">
      <p className="text-heading text-ink">No lyrics for this song</p>
      <p className="mt-1 text-ui text-ink-muted">
        Apple Music doesn't have lyrics for it in your storefront.
      </p>
    </div>
  );
}

/** A language tag's name in the UI's language ("en-US" → "English"). */
function languageName(tag: string): string {
  try {
    return new Intl.DisplayNames(undefined, { type: "language" }).of(tag.split("-")[0]) ?? tag;
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
  const toggle = "aria-pressed:bg-active aria-pressed:text-ink";
  return (
    <>
      {has.translation && (
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={prefs.translation}
          title={`Translation into ${languageName(lyrics.data?.translationLanguage ?? "")}`}
          onClick={() => prefs.set({ translation: !prefs.translation })}
          className={toggle}
        >
          <Languages strokeWidth={1.75} />
          Translation
        </Button>
      )}
      {has.pronunciation && (
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={prefs.pronunciation}
          title="The words in Latin letters"
          onClick={() => prefs.set({ pronunciation: !prefs.pronunciation })}
          className={toggle}
        >
          <SpellCheck strokeWidth={1.75} />
          Pronunciation
        </Button>
      )}
      {(has.translation || has.pronunciation) && (
        <span aria-hidden className="mx-1 h-4 w-px bg-line" />
      )}
    </>
  );
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

  useEffect(() => closeRef.current?.focus(), []);
  // Nothing to show once the queue is cleared.
  useEffect(() => {
    if (!current) closeOverlay();
  }, [current, closeOverlay]);
  if (!current) return null;

  const lyricsOn = showLyrics && !!current.catalogId;
  const cover = lyricsOn ? (narrow ? 200 : 320) : narrow ? 280 : 420;

  return (
    <section
      aria-label="Now playing"
      className="absolute inset-0 z-20 flex animate-in flex-col overflow-hidden bg-ground duration-150 fade-in"
    >
      <Backdrop src={current.backdrop} scrim={70} tone="ground" />
      <div className="relative flex h-12 shrink-0 items-center justify-end gap-1 px-4">
        {lyricsOn && <LyricsOptions catalogId={current.catalogId} />}
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={lyricsOn}
          disabled={!current.catalogId}
          onClick={() => setShowLyrics(!showLyrics)}
        >
          {lyricsOn ? <Disc3 strokeWidth={1.75} /> : <MicVocal strokeWidth={1.75} />}
          {lyricsOn ? "Cover only" : "Lyrics"}
        </Button>
        <Button
          ref={closeRef}
          variant="ghost"
          size="icon-sm"
          aria-label="Close now playing (Escape)"
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
          <Artwork src={current.art.large} size={cover} alt={`${current.albumName} cover`} eager />
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
