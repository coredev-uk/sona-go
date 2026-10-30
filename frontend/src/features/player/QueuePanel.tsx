import { PlaybackService, type QueueItem } from "@bindings/services";
import { Link } from "@tanstack/react-router";
import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useVirtualizer } from "@tanstack/react-virtual";
import { GripVertical, Infinity as InfinityIcon, X } from "lucide-react";
import {
  memo,
  useLayoutEffect,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
} from "react";

import { Artwork } from "@/shared/components/Artwork";
import { beginDrag, dropIndex, useDrag, useDropTarget, type DragPayload } from "@/shared/dnd/drag";
import { Equaliser } from "@/shared/tracks/Equaliser";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { formatTime } from "@/shared/lib/format";
import { run } from "@/shared/lib/run";
import { useWithheld } from "@/shared/lib/content-policy";
import { cn } from "@/shared/lib/utils";

import { queueItems } from "./commands";
import { usePlayback } from "./playback";
import { focusedRow, rowForKey, type QueueFocus } from "./queue-focus";
import { QUEUE_TOGGLE_ID, useUI } from "./ui";

const ROW = 52;
const HEAD = 44;
/** The queue's id as a drag origin and drop target. */
const QUEUE = "queue";

/** A line of the queue list: a song, or the heading of the station's part. */
type Line =
  | { kind: "row"; qi: number }
  | { kind: "head"; key: string; title: string; detail: string }
  | { kind: "note"; key: string; text: string };

interface RowProps {
  item: QueueItem;
  index: number;
  current: boolean;
  playing: boolean;
  /** The row is being dragged to a new place. */
  dragged: boolean;
  /** The song after the current one. */
  upNext: boolean;
  /** The list's Tab stop (roving tabindex). */
  focused: boolean;
}

/** The queue's keyboard help, read after each row's name. */
const KEYS_HELP = "queue-keys-help";

/** Memoised: a drag or track change re-renders only the rows whose props change. */
const QueueRow = memo(function QueueRow({
  item,
  index,
  current,
  playing,
  dragged,
  upNext,
  focused,
}: RowProps) {
  const len = usePlayback((s) => s.queue.items.length);
  // Skipped by Go on an account that restricts explicit content.
  const withheld = useWithheld(item.explicit);
  const { t } = useLingui();
  const name = item.name;
  const artist = item.artistName;
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void run(t`Couldn't play that song`, PlaybackService.Jump(index));
    } else if (e.key === "Delete" || e.key === "Backspace") {
      e.preventDefault();
      void run(t`Couldn't remove it`, PlaybackService.Remove(index));
    } else if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault();
      const to = index + (e.key === "ArrowUp" ? -1 : 1);
      if (to >= 0 && to < len) void run(t`Couldn't move it`, PlaybackService.Move(index, to));
    }
  };
  const label = withheld ? t`${name}, ${artist}, skipped: explicit` : t`${name}, ${artist}`;
  return (
    <div
      role="listitem"
      tabIndex={focused ? 0 : -1}
      aria-current={current || undefined}
      aria-label={upNext ? t`Up next: ${label}` : label}
      aria-describedby={current ? undefined : KEYS_HELP}
      title={
        withheld ? t`Explicit content is restricted on this account; it will be skipped` : undefined
      }
      data-qi={index}
      data-movable={(!current && !item.auto) || undefined}
      onDoubleClick={() => void run(t`Couldn't play that song`, PlaybackService.Jump(index))}
      onKeyDown={onKeyDown}
      className={cn(
        "group/q relative flex h-[52px] items-center gap-3 rounded-md px-2 transition-colors duration-100 select-none hover:bg-hover focus-visible:bg-hover focus-visible:outline-offset-[-2px]",
        dragged && "bg-active opacity-60",
        withheld && !dragged && "opacity-50",
      )}
    >
      {!current && !item.auto && (
        <GripVertical
          aria-hidden
          className="absolute left-0 size-3.5 cursor-grab text-ink-faint opacity-0 group-hover/q:opacity-100"
          strokeWidth={1.75}
        />
      )}
      <Artwork src={item.art.small} size={36} alt="" />
      <div className="min-w-0 flex-1">
        <p className={cn("truncate text-ui", current ? "font-medium text-play-ink" : "text-ink")}>
          {item.name}
        </p>
        <p className="truncate text-sub text-ink-muted">{item.artistName}</p>
      </div>
      {current ? (
        <Equaliser playing={playing} className="mr-1.5" />
      ) : (
        <>
          <span className="text-meta text-ink-faint num group-focus-within/q:hidden group-hover/q:hidden">
            {formatTime(item.durationMs)}
          </span>
          <Button
            variant="ghost"
            size="icon-xs"
            tabIndex={-1}
            aria-label={t`Remove ${name} from the queue`}
            onClick={() => void run(t`Couldn't remove it`, PlaybackService.Remove(index))}
            className="hidden text-ink-muted group-focus-within/q:inline-flex group-hover/q:inline-flex hover:text-ink"
          >
            <X />
          </Button>
        </>
      )}
    </div>
  );
});

/**
 * The queue's virtual lines. The virtualizer stays in this hook (React
 * Compiler skips it); the panel renders from `items` and `totalSize`, which
 * change only when the visible range or the line positions do, and uses
 * `virtualizer` only in event handlers and effects (`scrollToIndex`).
 */
function useQueueLines(scrollRef: RefObject<HTMLDivElement | null>, lines: readonly Line[]) {
  // eslint-disable-next-line react-hooks/incompatible-library -- useVirtualizer is isolated in this hook on purpose; the compiler skips it
  const v = useVirtualizer({
    count: lines.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (i) => (lines[i].kind === "row" ? ROW : HEAD),
    overscan: 8,
  });
  return { virtualizer: v, items: v.getVirtualItems(), totalSize: v.getTotalSize() };
}

/**
 * The queue beside the page: the current song, then what plays next, the
 * user's songs first and then, under their own heading, what the station
 * or AutoPlay adds. The list is one Tab stop: ↑/↓/Home/End move between
 * songs, drag or Alt+↑/↓ reorders, Delete removes, Enter or double-click
 * jumps, Escape closes the panel. Songs, albums and playlists dragged from
 * a page drop into the gap under the pointer (or after the user's songs).
 */
export function QueuePanel() {
  const items = usePlayback((s) => s.queue.items);
  // The state's index moves with every track; the queue's only when the queue changes.
  const index = usePlayback((s) => s.index);
  const station = usePlayback((s) => s.queue.station);
  const autoPlay = usePlayback((s) => s.autoPlay);
  const repeat = usePlayback((s) => s.repeat);
  const playing = usePlayback((s) => s.state === "playing" || s.state === "loading");
  const close = useUI((s) => s.setQueueOpen);
  const scrollRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const dragFrom = useDrag((s) =>
    s.active?.origin?.list === QUEUE ? s.active.origin.index : null,
  );
  /** The gap a drop would land in (before queue[slot]); null when not over the list. */
  const [slot, setSlot] = useState<number | null>(null);
  const { t } = useLingui();

  // The current song first, then the rest of the queue; earlier songs are played.
  const start = Math.max(0, index);
  const lines: Line[] = [];
  let mine = 0;
  let auto = 0;
  for (let qi = start; qi < items.length; qi++) {
    const it = items[qi];
    // The station's part gets a heading when it follows songs the user
    // queued, and always for AutoPlay (it is not what the user chose).
    if (qi > start && it.auto && auto === 0 && station && (station.autoPlay || mine > 0)) {
      const stationName = station.name;
      lines.push({
        kind: "head",
        key: "head",
        title: station.autoPlay ? t`AutoPlay` : t`Then ${stationName}`,
        detail: station.autoPlay
          ? t`Similar music, from ${stationName}`
          : t`The station continues after your songs`,
      });
    }
    if (qi > start) {
      if (it.auto) auto++;
      else mine++;
    }
    lines.push({ kind: "row", qi });
  }
  // AutoPlay needs a song to continue from; radio streams ("ra.…") are not songs.
  const continues =
    autoPlay && !station && repeat === "off" && items.some((i) => !i.id.startsWith("ra."));
  if (continues) {
    lines.push({ kind: "note", key: "note", text: t`Then AutoPlay continues with similar music` });
  }

  const { virtualizer, items: rows, totalSize } = useQueueLines(scrollRef, lines);

  // Roving focus: one row is the list's Tab stop and the arrows move it. The
  // stop must be rendered, so while the focused row is scrolled out of the
  // virtual range the first rendered row holds it.
  const [focus, setFocus] = useState<QueueFocus | null>(null);
  const focusQi = focusedRow(items, start, focus);
  let tabQi = -1;
  for (const r of rows) {
    const l = lines[r.index];
    if (l.kind !== "row") continue;
    if (l.qi === focusQi) {
      tabQi = l.qi;
      break;
    }
    if (tabQi < 0) tabQi = l.qi;
  }
  /** Scrolls row `qi` into the virtual range and focuses it once rendered. */
  const showRow = (qi: number) => {
    const line = lines.findIndex((l) => l.kind === "row" && l.qi === qi);
    if (line < 0) return;
    virtualizer.scrollToIndex(line, { align: "auto" });
    const go = (tries: number) => {
      const el = scrollRef.current?.querySelector<HTMLElement>(`[data-qi="${qi}"]`);
      if (el) el.focus({ preventScroll: true });
      else if (tries < 10) requestAnimationFrame(() => go(tries + 1));
    };
    requestAnimationFrame(() => go(0));
  };
  // The row that has focus. When its song leaves the list with focus on it
  // (removed, played), focus goes to the row that takes its place, not to
  // <body>. A row the virtualizer unmounts while scrolling is not followed.
  const focusedEl = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    const el = focusedEl.current;
    if (!el || el.isConnected) return;
    focusedEl.current = null;
    const listed = focus !== null && items.findIndex((it) => it.uid === focus.uid) >= start;
    const active = document.activeElement;
    if (listed || focusQi < 0 || (active && active !== document.body)) return;
    showRow(focusQi);
  });
  const onListFocus = (e: FocusEvent<HTMLDivElement>) => {
    const row = e.target.closest<HTMLElement>("[data-qi]");
    if (!row) return;
    focusedEl.current = row;
    const qi = Number(row.dataset.qi);
    const uid = items[qi]?.uid;
    if (uid && (focus?.uid !== uid || focus.qi !== qi)) setFocus({ uid, qi });
  };
  const onListBlur = (e: FocusEvent<HTMLDivElement>) => {
    const row = focusedEl.current;
    if (!row || (e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget))) {
      return;
    }
    // Focus left the list, unless the row was removed under it (known once the DOM settles).
    setTimeout(() => {
      if (row.isConnected && focusedEl.current === row) focusedEl.current = null;
    }, 0);
  };
  const onListKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    const row = e.target instanceof Element ? e.target.closest<HTMLElement>("[data-qi]") : null;
    if (!row) return;
    const to = rowForKey(e.key, Number(row.dataset.qi), start, items.length);
    if (to === null) return;
    e.preventDefault();
    showRow(to);
  };
  // Escape anywhere in the panel closes it, back on the bar's queue toggle.
  const onPanelKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key !== "Escape" || e.defaultPrevented) return;
    e.preventDefault();
    close(false);
    document.getElementById(QUEUE_TOGGLE_ID)?.focus();
  };

  // The user's part ends at the first station slot; drops land inside it.
  let userEnd = items.length;
  for (let qi = start + 1; qi < items.length; qi++) {
    if (items[qi].auto) {
      userEnd = qi;
      break;
    }
  }
  // Nothing goes before the current song (with none, anywhere).
  const first = Math.min(index + 1, items.length);

  // Line tops (fixed heights), to place the drop line and map the pointer to a gap.
  const tops: number[] = [];
  let y = 0;
  for (const l of lines) {
    tops.push(y);
    y += l.kind === "row" ? ROW : HEAD;
  }
  const slotAt = (clientY: number): number => {
    const top = bodyRef.current?.getBoundingClientRect().top ?? 0;
    const off = clientY - top;
    let s = userEnd;
    for (let i = 0; i < lines.length; i++) {
      const l = lines[i];
      if (off < tops[i] + (l.kind === "row" ? ROW / 2 : 0)) {
        s = l.kind === "row" ? l.qi : userEnd;
        break;
      }
      if (l.kind === "row" && off < tops[i] + ROW) {
        s = l.qi + 1;
        break;
      }
    }
    return Math.max(first, Math.min(userEnd, s));
  };
  const slotTop = (s: number) => {
    const i = lines.findIndex((l) => l.kind !== "row" || l.qi >= s);
    return i < 0 ? totalSize : tops[i];
  };

  const accepts = (p: DragPayload) =>
    p.origin?.list === QUEUE ||
    p.items.some((i) => i.kind === "song" || i.kind === "album" || i.kind === "playlist");
  const over = useDropTarget(QUEUE, {
    accepts,
    describe: (p) => (p.origin?.list === QUEUE ? t`Move here` : t`Add to the queue`),
    hover: (_, at) => {
      const s = at && items.length ? slotAt(at.y) : null;
      setSlot((prev) => (prev === s ? prev : s));
    },
    drop: (p, at) => {
      const s = items.length ? slotAt(at.y) : null;
      if (p.origin?.list === QUEUE) {
        const from = p.origin.index;
        const to = s === null ? from : dropIndex(from, s);
        if (to !== from) void run(t`Couldn't move it`, PlaybackService.Move(from, to));
      } else {
        void queueItems(p.items, s ?? "last");
      }
    },
  });
  // A row that does not move (its own gap) shows no line.
  const showSlot =
    over && slot !== null && (dragFrom === null || (slot !== dragFrom && slot !== dragFrom + 1));

  const grab = (e: PointerEvent<HTMLDivElement>) => {
    if (!(e.target instanceof Element) || e.target.closest("button, a")) return;
    const row = e.target.closest<HTMLElement>("[data-movable]");
    if (!row) return;
    const qi = Number(row.dataset.qi);
    const it = items[qi];
    beginDrag(e, () => ({
      items: [],
      origin: { list: QUEUE, index: qi },
      label: it.name,
      art: it.art.small,
    }));
  };

  const playingFrom = station?.name;
  const summary = !items.length ? (
    ""
  ) : items[start]?.live && !mine ? (
    t`Live radio`
  ) : mine ? (
    <Plural value={mine} one="# song after this one" other="# songs after this one" />
  ) : auto ? (
    t`Nothing queued after this song`
  ) : (
    t`Nothing after this song`
  );

  return (
    <aside
      aria-label={t`Queue`}
      data-drop={QUEUE}
      className="flex h-full w-[320px] shrink-0 flex-col border-l border-line bg-ground"
      onKeyDown={onPanelKeyDown}
    >
      <div className="flex h-12 shrink-0 items-center justify-between gap-2 pr-2 pl-4">
        <div className="min-w-0">
          <h2 className="text-heading text-ink">
            <Trans>Queue</Trans>
          </h2>
        </div>
        <div className="flex items-center gap-1">
          {mine > 0 && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void run(t`Couldn't clear the queue`, PlaybackService.Clear())}
            >
              <Trans>Clear</Trans>
            </Button>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t`AutoPlay`}
                aria-pressed={autoPlay}
                onClick={() => {
                  usePlayback.getState().patch({ autoPlay: !autoPlay });
                  void run(t`Couldn't change AutoPlay`, PlaybackService.SetAutoPlay(!autoPlay));
                }}
                className={cn(
                  "relative text-ink-muted hover:text-ink",
                  autoPlay &&
                    "text-ink after:absolute after:bottom-0.5 after:left-1/2 after:size-1 after:-translate-x-1/2 after:rounded-full after:bg-ink",
                )}
              >
                <InfinityIcon strokeWidth={1.75} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {autoPlay ? t`AutoPlay on: similar music plays when the queue ends` : t`AutoPlay off`}
            </TooltipContent>
          </Tooltip>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t`Close queue`}
            onClick={() => close(false)}
          >
            <X strokeWidth={1.75} />
          </Button>
        </div>
      </div>
      {items.length === 0 ? (
        <div
          className={cn(
            "mx-4 mt-2 rounded-lg border border-dashed border-control-line px-4 py-5 transition-colors duration-100",
            over && "border-ink/40 bg-hover",
          )}
        >
          <p className="text-ui font-medium text-ink">
            {over ? t`Drop to start a queue` : t`The queue is empty`}
          </p>
          <p className="mt-1 text-sub text-ink-muted">
            <Trans>
              Play an album, playlist or station, choose Play next on any song, or drag songs here.
            </Trans>
          </p>
        </div>
      ) : (
        <>
          <p className="shrink-0 truncate px-4 pb-2 text-meta text-ink-muted num">
            {station && !station.autoPlay ? (
              <Trans>
                Playing from{" "}
                <Link
                  to="/station/$id"
                  params={{ id: station.id }}
                  className="text-ink underline decoration-ink/40 underline-offset-2 hover:decoration-ink"
                >
                  {playingFrom}
                </Link>
              </Trans>
            ) : (
              summary
            )}
          </p>
          <p id={KEYS_HELP} className="sr-only">
            <Trans>
              Enter plays the song, Delete removes it, Alt+Up and Alt+Down move it, Escape closes
              the queue.
            </Trans>
          </p>
          <div
            ref={scrollRef}
            role="list"
            aria-label={t`Queue`}
            className="min-h-0 flex-1 overflow-y-auto px-2 pb-3"
            onPointerDown={grab}
            onFocus={onListFocus}
            onBlur={onListBlur}
            onKeyDown={onListKeyDown}
          >
            <div ref={bodyRef} className="relative" style={{ height: totalSize }}>
              {rows.map((row) => {
                const line = lines[row.index];
                const style = { transform: `translateY(${row.start}px)` };
                if (line.kind !== "row") {
                  return (
                    <div
                      key={line.key}
                      role="listitem"
                      className="absolute inset-x-0 top-0 flex flex-col justify-end px-2 pb-1.5"
                      style={{ ...style, height: HEAD }}
                    >
                      {line.kind === "head" ? (
                        <>
                          <h3 className="text-ui font-medium text-ink">{line.title}</h3>
                          <p className="truncate text-meta text-ink-muted">{line.detail}</p>
                        </>
                      ) : (
                        <p className="flex items-center gap-1.5 text-meta text-ink-muted">
                          <InfinityIcon aria-hidden className="size-3.5" strokeWidth={1.75} />
                          {line.text}
                        </p>
                      )}
                    </div>
                  );
                }
                const qi = line.qi;
                const item: QueueItem = items[qi];
                return (
                  <div key={item.uid} className="absolute inset-x-0 top-0" style={style}>
                    <QueueRow
                      item={item}
                      index={qi}
                      current={qi === index}
                      playing={playing}
                      dragged={dragFrom === qi}
                      upNext={qi === start + 1}
                      focused={qi === tabQi}
                    />
                  </div>
                );
              })}
              {showSlot && (
                <div
                  aria-hidden
                  className="pointer-events-none absolute inset-x-2 z-10 h-0.5 -translate-y-px rounded-full bg-ink"
                  style={{ top: slotTop(slot) }}
                />
              )}
            </div>
          </div>
        </>
      )}
    </aside>
  );
}
