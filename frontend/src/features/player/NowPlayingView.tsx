import {
  PlaybackService,
  type BroadcasterInfo,
  type LyricLine,
  type QueueItem,
} from "@bindings/services";
import { Link } from "@tanstack/react-router";
import { Trans, useLingui } from "@lingui/react/macro";
import { Browser } from "@wailsio/runtime";
import { Clapperboard, Info, Languages, Maximize2, SpellCheck, X } from "lucide-react";
import {
  memo,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from "react";

import { useOnAirProgramme } from "@/features/radio";
import { FluidArtwork } from "@/shared/components/Artwork";
import { Backdrop } from "@/shared/components/Backdrop";
import { ExplicitMark, PreviewMark } from "@/shared/components/Badges";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { MotionCover } from "@/shared/components/MotionCover";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { onAirLine } from "@/shared/items/station";
import { callFailure } from "@/shared/lib/errors";
import { clockTime } from "@/shared/lib/format";
import { useMotionAllowed } from "@/shared/lib/motion";
import { run } from "@/shared/lib/run";
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
import { LyricBody, type LyricLangs } from "./LyricBody";
import { hasWordTiming, lineVoices, type Voice } from "./lyric-words";
import { LyricsTiming } from "./LyricsTiming";
import { lyricsSongId, usePlayback } from "./playback";
import { useAlbumMotion, useLyrics } from "./queries";
import { QueueList, QueueSummary, QueueTools } from "./QueuePanel";
import { ROOM_TOGGLE_ID, shownPane, useUI, type Pane } from "./ui";
import {
  lineTone,
  useLyricLangs,
  useSungLine,
  useTurnPage,
  useVerses,
  type LineState,
} from "./useLyricsView";
import { useWordSync } from "./useWordSync";

const Line = memo(function Line({
  songId,
  index,
  line,
  state,
  prefs,
  langs,
  voice,
  together,
  tabbable,
  onFocusLine,
}: {
  songId: string;
  index: number;
  line: LyricLine;
  state: LineState;
  prefs: LyricsPrefs;
  langs: LyricLangs;
  voice: Voice;
  /** The first line of a passage every voice sings: it carries the "Together" cue. */
  together: boolean;
  tabbable: boolean;
  onFocusLine: (index: number) => void;
}) {
  const { t } = useLingui();
  const body = (
    <>
      {together && (
        <span className="mb-0.5 block text-meta font-medium text-ink-muted">
          <Trans>Together</Trans>
        </span>
      )}
      <LyricBody
        shown={shownLine(line, prefs)}
        langs={langs}
        subClassName="mt-0.5 text-heading text-ink-muted"
      />
    </>
  );
  // No colour transition between states: each line change would draw a
  // dozen frames (docs/frontend-architecture.md, "Word-timed lyrics").
  const cls = cn(
    "block w-full rounded-md px-3 py-1.5 text-start text-page text-balance",
    lineTone(state, line.words.length > 0),
    // A duet's other voice, as a lyric sheet sets it: indented, in italic.
    voice === "two" && "ps-[calc(0.75rem+1.5em)] italic",
  );
  if (state === "plain") {
    return (
      <p dir="auto" className={cls}>
        {body}
      </p>
    );
  }
  return (
    <button
      type="button"
      dir="auto"
      data-line={index}
      data-active={state === "active" || undefined}
      aria-current={state === "active" ? "true" : undefined}
      tabIndex={tabbable ? 0 : -1}
      onFocus={() => onFocusLine(index)}
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

/** The keys that move between synced lines: one Tab stop, the arrows inside. */
const LINE_KEYS: Record<string, (i: number, last: number) => number> = {
  ArrowDown: (i, last) => Math.min(i + 1, last),
  ArrowUp: (i) => Math.max(i - 1, 0),
  Home: () => 0,
  End: (_, last) => last,
};

function LyricsPane({ catalogId }: { catalogId: string }) {
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
  const [focused, setFocused] = useState(-1);
  const { t } = useLingui();

  if (lyrics.isPending) {
    return (
      <div aria-busy className="flex flex-col gap-5 px-3 pt-[14cqh]">
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
  // The line Tab lands on: the one focused last, else the one being sung.
  const tabStop = focused >= 0 ? focused : Math.max(anchor, 0);

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const to = LINE_KEYS[e.key];
    if (!to || !synced) return;
    const buttons = [...e.currentTarget.querySelectorAll<HTMLElement>("button[data-line]")];
    const i = buttons.findIndex((b) => b === document.activeElement);
    if (i < 0) return;
    e.preventDefault();
    userScrolled();
    buttons[to(i, buttons.length - 1)].focus();
  };

  return (
    <div
      ref={scrollRef}
      role="group"
      aria-label={t`Lyrics`}
      // Plain lyrics hold no buttons: the sheet itself takes focus to scroll.
      tabIndex={synced ? undefined : 0}
      onWheel={userScrolled}
      onTouchMove={userScrolled}
      onKeyDown={onKeyDown}
      className="no-scrollbar h-full overflow-y-auto lyrics-fade pe-2 pt-[14cqh] pb-[70cqh] focus-visible:outline-offset-[-2px]"
    >
      {verses.map((verse, v) => (
        <div key={v} data-verse className={cn(v > 0 && "mt-8")}>
          {verse.map((i) => (
            <Line
              key={i}
              songId={catalogId}
              index={i}
              line={lines[i]}
              state={
                !synced
                  ? "plain"
                  : i === active
                    ? "active"
                    : i < Math.max(active, anchor)
                      ? "past"
                      : "future"
              }
              prefs={prefs}
              langs={langs}
              voice={voices[i]}
              together={duet && voices[i] === "all" && voices[i - 1] !== "all"}
              tabbable={i === tabStop}
              onFocusLine={setFocused}
            />
          ))}
        </div>
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
  const closeOverlay = useUI((s) => s.closeOverlay);
  return (
    <div className="mt-[14cqh] max-w-md rounded-lg border border-dashed border-control-line px-5 py-5">
      <p className="text-heading text-ink">
        <Trans>No lyrics for this song</Trans>
      </p>
      <p className="mt-1 text-ui text-ink-muted">
        <Trans>None of the lyrics sources in Settings has lyrics for it.</Trans>
      </p>
      <Link
        to="/settings"
        search={{ section: "playback" }}
        onClick={closeOverlay}
        className="mt-3 inline-block text-ui font-medium text-ink underline decoration-ink/40 underline-offset-2 hover:decoration-ink"
      >
        <Trans>Lyrics sources</Trans>
      </Link>
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
 * The lyric sheet's own controls, in a strip at its foot as full screen
 * keeps them: translation and pronunciation at the start (only when the
 * lyrics have them for this song; each adds its line under every sung
 * line, and the choice is kept for every song), then, for synced lyrics,
 * the timing nudge and full screen at the end.
 */
function LyricsTools({ catalogId }: { catalogId: string }) {
  const lyrics = useLyrics(catalogId);
  const prefs = useLyricsPrefs();
  const openRoom = useUI((s) => s.openRoom);
  const roomKeys = useBinding("player.lyrics-room");
  const has = lyricsExtras(lyrics.data);
  const synced = isSynced(lyrics.data) && !!lyrics.data?.lines.length;
  const { t, i18n } = useLingui();
  const language = languageName(lyrics.data?.translationLanguage ?? "", i18n.locale);
  const TOGGLE = "aria-pressed:bg-ink/10 aria-pressed:text-ink";
  if (!has.translation && !has.pronunciation && !synced) return null;
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 ps-0.5 pt-3">
      {(has.translation || has.pronunciation) && (
        <div className="flex items-center gap-1">
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
        </div>
      )}
      {synced && (
        <div className="ms-auto flex items-center gap-1">
          <LyricsTiming songId={catalogId} />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button id={ROOM_TOGGLE_ID} variant="ghost" size="sm" onClick={openRoom}>
                <Maximize2 strokeWidth={1.75} />
                <Trans>Full screen</Trans>
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              <Trans>Lyrics across the whole screen, large enough to read across a room</Trans>
              {roomKeys[0] && <ChordKeys chord={roomKeys[0]} className="ml-1.5" />}
            </TooltipContent>
          </Tooltip>
        </div>
      )}
    </div>
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
  // One rendition for every size, so a resize never reloads the video.
  return <MotionCover motion={motion.data} art={art} size={size} videoSize={420} alt={alt} eager />;
}

/**
 * How the view is laid out in its box: the cover beside the pane (`cover`
 * from a fixed set, so Go serves the art at its exact size), or, in a
 * narrow column, the song as a header over the pane. Measured, not from
 * the window: the sidebar takes 232px of it or none. Re-renders only when
 * the layout changes, never while a resize stays within one.
 */
function useNowPlayingLayout(ref: RefObject<HTMLElement | null>) {
  const [layout, setLayout] = useState({ stacked: false, cover: 320 });
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => {
      const { width, height } = el.getBoundingClientRect();
      const stacked = width < 880;
      const byWidth = width >= 1000 ? 400 : 320;
      const byHeight = height >= 720 ? 400 : height >= 640 ? 320 : 240;
      const cover = stacked ? 168 : Math.min(byWidth, byHeight);
      setLayout((l) => (l.stacked === stacked && l.cover === cover ? l : { stacked, cover }));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  return layout;
}

/** A link out of the view: it closes the view as it goes. */
function OutLink({
  to,
  id,
  children,
}: {
  to: "/artist/$id" | "/album/$id";
  id: string;
  children: ReactNode;
}) {
  const closeOverlay = useUI((s) => s.closeOverlay);
  return (
    <Link
      to={to}
      params={{ id }}
      onClick={closeOverlay}
      className="hover:text-ink hover:underline hover:underline-offset-2"
    >
      {children}
    </Link>
  );
}

/**
 * A live station's line: Live, and what is on air (the stream's own words,
 * else the schedule), unless the heading already says it (`onAirInHeading`).
 */
function LiveLines({ id, onAirInHeading }: { id: string; onAirInHeading?: boolean }) {
  const playing = usePlayback((s) => s.state === "playing" || s.state === "loading");
  const onAir = usePlayback((s) => s.onAir);
  const programme = useOnAirProgramme(id);
  const until = programme ? clockTime(programme.endMs) : "";
  return (
    <>
      <p className="mt-2 flex items-center gap-2 text-heading font-medium text-ink">
        <span
          aria-hidden
          className={cn("size-2 rounded-full", playing ? "bg-play" : "bg-ink-faint")}
        />
        <Trans>Live</Trans>
      </p>
      {onAirInHeading ? null : onAir ? (
        <p className="mt-1 line-clamp-2 text-sub text-ink-muted">{onAirLine(onAir)}</p>
      ) : (
        programme && (
          <p className="mt-1 line-clamp-2 text-sub text-ink-muted">
            {programme.title}
            <span className="num">
              {" · "}
              <Trans>until {until}</Trans>
            </span>
          </p>
        )
      )}
    </>
  );
}

/**
 * What the view says about a local broadcaster under the song on air: the
 * station's name with its genre and place, its description (TuneIn's, for
 * the stations it serves) and a link to its website.
 */
function BroadcasterBlock({ name, info }: { name: string; info: BroadcasterInfo | null }) {
  const line = [name, info?.genre, info?.location].filter(Boolean).join(" · ");
  return (
    <div className="mt-4 min-w-0 text-sub text-ink-muted">
      <p className="truncate">
        <span className="font-medium text-ink">
          <Trans>On air</Trans>
        </span>
        {" · "}
        {line}
      </p>
      {info?.description && <p className="mt-1 line-clamp-3 text-pretty">{info.description}</p>}
      {info?.website && (
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={() => void Browser.OpenURL(info.website)}
        >
          <Trans>Open website</Trans>
        </Button>
      )}
    </div>
  );
}

/**
 * The song's label copy under (or, narrow, beside) the cover: the title,
 * the artist and the album as links out, the marks the bar also shows
 * (Explicit, Preview), and the song's own actions: Favourite, Suggest Less
 * and its credits. A local broadcaster is headed by the song its stream
 * names on air; a music video offers its picture.
 */
function SongCopy({ current, large }: { current: QueueItem; large: boolean }) {
  const preview = usePlayback((s) => s.preview);
  const onAir = usePlayback((s) => s.onAir);
  const broadcaster = usePlayback((s) => s.broadcaster);
  const closeOverlay = useUI((s) => s.closeOverlay);
  const openVideo = useUI((s) => s.openVideo);
  const rated = !current.live && !current.video && !!(current.catalogId || current.libraryId);
  const broadcast = current.live && current.broadcast;
  const heading = broadcast && onAir ? onAir.title : current.name;
  const byline = broadcast && onAir ? onAir.artist : current.artistName;
  return (
    <div className="min-w-0">
      <h2
        className={cn("line-clamp-3 text-balance text-ink", large ? "text-display" : "text-page")}
      >
        {heading}
      </h2>
      {current.live ? (
        <>
          {broadcast && byline && (
            <p className="mt-2 truncate text-heading font-medium text-ink-muted">{byline}</p>
          )}
          <LiveLines id={current.id} onAirInHeading={broadcast && !!onAir} />
          {broadcast && <BroadcasterBlock name={current.name} info={broadcaster} />}
        </>
      ) : (
        <>
          {current.artistName && (
            <p className="mt-2 truncate text-heading font-medium text-ink-muted">
              {current.artistId ? (
                <OutLink to="/artist/$id" id={current.artistId}>
                  {current.artistName}
                </OutLink>
              ) : (
                current.artistName
              )}
            </p>
          )}
          {current.albumName && (
            <p className="mt-0.5 truncate text-sub text-ink-muted">
              {current.albumId ? (
                <OutLink to="/album/$id" id={current.albumId}>
                  {current.albumName}
                </OutLink>
              ) : (
                current.albumName
              )}
            </p>
          )}
          {(current.explicit || preview) && (
            <div className="mt-2.5 flex items-center gap-2">
              {current.explicit && <ExplicitMark />}
              {preview && <PreviewMark />}
            </div>
          )}
        </>
      )}
      {current.video && (
        <Button variant="outline" size="sm" className="mt-3" onClick={openVideo}>
          <Clapperboard strokeWidth={1.75} />
          <Trans>Show video</Trans>
        </Button>
      )}
      {rated && (
        <div className="-ms-2 mt-3 flex items-center gap-1">
          <RatingButtons kind="songs" item={current} name={current.name} />
          {current.catalogId && (
            <Button variant="ghost" size="sm" asChild className="text-ink-muted hover:text-ink">
              <Link to="/song/$id" params={{ id: current.catalogId }} onClick={closeOverlay}>
                <Info strokeWidth={1.75} />
                <Trans>View credits</Trans>
              </Link>
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

const PANES: readonly Pane[] = ["lyrics", "queue"];
const tabId = (p: Pane) => `now-playing-tab-${p}`;
const PANEL_ID = "now-playing-pane";

/**
 * The switch between the panes: tabs named in words, the shown one in ink
 * over a 2px ink rule (amber stays for what plays). One Tab stop; ← and →
 * move and show, Home and End go to the ends. With one pane (a station, a
 * song outside the catalogue) it is only that pane's heading.
 */
function PaneTabs({ panes, shown }: { panes: readonly Pane[]; shown: Pane }) {
  const setPane = useUI((s) => s.setPane);
  const { t } = useLingui();
  const names: Record<Pane, string> = { lyrics: t`Lyrics`, queue: t`Up next` };
  // The pane changed from outside the tabs (the bar's Queue or Lyrics, Q):
  // a focused tab follows it, so the focus ring never sits on the pane not shown.
  useEffect(() => {
    const focused = document.activeElement;
    if (focused?.getAttribute("role") === "tab" && focused.id !== tabId(shown))
      document.getElementById(tabId(shown))?.focus();
  }, [shown]);
  if (panes.length < 2) {
    return <h3 className="text-heading leading-10 text-ink">{names[shown]}</h3>;
  }
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = panes.indexOf(shown);
    const to =
      e.key === "ArrowRight"
        ? (i + 1) % panes.length
        : e.key === "ArrowLeft"
          ? (i - 1 + panes.length) % panes.length
          : e.key === "Home"
            ? 0
            : e.key === "End"
              ? panes.length - 1
              : -1;
    if (to < 0) return;
    e.preventDefault();
    setPane(panes[to]);
    document.getElementById(tabId(panes[to]))?.focus();
  };
  return (
    <div role="tablist" className="flex items-center gap-6" onKeyDown={onKeyDown}>
      {panes.map((p) => (
        <button
          key={p}
          type="button"
          role="tab"
          id={tabId(p)}
          aria-selected={p === shown}
          aria-controls={PANEL_ID}
          tabIndex={p === shown ? 0 : -1}
          onClick={() => setPane(p)}
          className={cn(
            "relative -mx-1.5 h-10 rounded-sm px-1.5 text-heading text-ink-muted transition-colors duration-100 hover:text-ink",
            "after:absolute after:inset-x-1.5 after:bottom-0.5 after:h-0.5 after:rounded-full",
            "aria-selected:text-ink aria-selected:after:bg-ink",
          )}
        >
          {names[p]}
        </button>
      ))}
    </div>
  );
}

/** The view's Close button: focus starts there when the view has one pane. */
const CLOSE_ID = "now-playing-close";

/**
 * The now-playing view over the page column: the record and its insert.
 * The cover on a wash of its colour, the song's label copy under it, and
 * beside it one pane at a time, switched by tabs: the lyric sheet, or Up
 * next (the queue). Each pane's own controls sit in a strip at its foot;
 * Close ends the tab row. Under 880px of column the song becomes a header
 * over the pane.
 * Escape or Close returns to the page.
 */
export function NowPlayingView() {
  const current = usePlayback((s) => s.current);
  const onAir = usePlayback((s) => s.onAir);
  const pane = useUI((s) => s.pane);
  const closeOverlay = useUI((s) => s.closeOverlay);
  const ref = useRef<HTMLElement>(null);
  const { stacked, cover } = useNowPlayingLayout(ref);
  const { t } = useLingui();
  const shown = shownPane(pane, current);

  // Focus starts on the shown pane's tab (Close when there is one pane), or
  // on Full screen when coming back from it.
  useEffect(() => {
    const el = ref.current;
    const toggle = useUI.getState().fromRoom ? document.getElementById(ROOM_TOGGLE_ID) : null;
    const tab = el?.querySelector<HTMLElement>("[role=tab][aria-selected=true]");
    (toggle ?? tab ?? el?.querySelector<HTMLElement>(`#${CLOSE_ID}`))?.focus();
  }, []);
  // Nothing to show once the queue is cleared.
  useEffect(() => {
    if (!current) closeOverlay();
  }, [current, closeOverlay]);

  const songId = lyricsSongId(current);
  const panes = songId && !current?.live ? PANES : (["queue"] as const);
  const albumName = current?.albumName;
  const close = (
    <Button
      id={CLOSE_ID}
      variant="ghost"
      size="icon-sm"
      aria-label={t`Close now playing (Escape)`}
      onClick={closeOverlay}
      className="shrink-0"
    >
      <X strokeWidth={1.75} />
    </Button>
  );
  // A music video's frame is 16:9, as wide as a cover; a local broadcaster
  // shows the cover of the song its stream names on air.
  const art =
    current &&
    (current.video ? (
      <div style={{ width: cover }}>
        <FluidArtwork src={current.art.large} size={cover} alt="" aspect="video" />
      </div>
    ) : (
      <NowPlayingCover
        albumId={current.albumId}
        art={(current.live && current.broadcast && onAir?.art.large) || current.art.large}
        size={cover}
        alt={albumName ? t`${albumName} cover` : ""}
      />
    ));

  return (
    <section
      ref={ref}
      aria-label={t`Now playing`}
      className="absolute inset-0 z-20 flex animate-in flex-col overflow-hidden bg-ground duration-150 fade-in"
    >
      {current && (
        <>
          <Backdrop src={current.backdrop} tone="ground" />
          <div
            className={cn(
              "relative mx-auto flex min-h-0 w-full max-w-[1280px] flex-1",
              stacked ? "flex-col px-6 pt-5 pb-4" : "gap-14 px-10 pt-6 pb-8",
            )}
          >
            {stacked ? (
              <div className="flex shrink-0 items-start gap-5 pb-5">
                {art}
                <div className="min-w-0 flex-1 pt-1">
                  <SongCopy current={current} large={false} />
                </div>
                {close}
              </div>
            ) : (
              <div
                className="flex shrink-0 flex-col gap-6 overflow-hidden"
                style={{ width: cover }}
              >
                {art}
                <SongCopy current={current} large={cover >= 320} />
              </div>
            )}
            <div className="flex min-h-0 min-w-0 flex-1 flex-col">
              {/* The column's text edge is 12px in, where the lyric lines,
                  the queue's covers and the tabs all start. */}
              <div className="flex shrink-0 items-center gap-6 ps-3">
                <PaneTabs panes={panes} shown={shown} />
                {!stacked && <div className="ms-auto">{close}</div>}
              </div>
              <div
                id={PANEL_ID}
                role={panes.length > 1 ? "tabpanel" : undefined}
                aria-labelledby={panes.length > 1 ? tabId(shown) : undefined}
                data-drop={shown === "queue" ? "queue" : undefined}
                className="mt-3 flex min-h-0 flex-1 flex-col"
              >
                {shown === "lyrics" ? (
                  <>
                    {/* A definite box for the sheet, which scrolls inside it;
                        a size container, so the first line starts at the top
                        of the reading band (14% of the sheet's own height)
                        however tall the window or the header above it. */}
                    <div className="relative min-h-0 flex-1">
                      <div className="[container-type:size] absolute inset-0">
                        <LyricsPane key={songId} catalogId={songId} />
                      </div>
                    </div>
                    <LyricsTools catalogId={songId} />
                  </>
                ) : (
                  <>
                    <QueueList inView />
                    <div className="flex shrink-0 items-center gap-4 ps-3 pt-3">
                      <QueueSummary onLeave={closeOverlay} className="min-w-0" />
                      <div className="ms-auto">
                        <QueueTools />
                      </div>
                    </div>
                  </>
                )}
              </div>
            </div>
          </div>
        </>
      )}
    </section>
  );
}
