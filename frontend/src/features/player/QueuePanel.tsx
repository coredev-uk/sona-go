import { PlaybackService, type QueueItem } from "@bindings/services";
import { Link } from "@tanstack/react-router";
import { useVirtualizer } from "@tanstack/react-virtual";
import { GripVertical, Infinity as InfinityIcon, X } from "lucide-react";
import { useRef, useState, type DragEvent, type KeyboardEvent } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { Equaliser } from "@/shared/tracks/Equaliser";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { formatTime, plural } from "@/shared/lib/format";
import { run } from "@/shared/lib/run";
import { cn } from "@/shared/lib/utils";

import { usePlayback } from "./playback";
import { useUI } from "./ui";

const ROW = 52;
const HEAD = 44;

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
  dragOver: boolean;
  onDragStart: (index: number) => void;
  onDragOver: (index: number) => void;
  onDrop: (index: number) => void;
}

function QueueRow({
  item,
  index,
  current,
  playing,
  dragOver,
  onDragStart,
  onDragOver,
  onDrop,
}: RowProps) {
  const len = usePlayback((s) => s.queue.items.length);
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void run("Couldn't play that song", PlaybackService.Jump(index));
    } else if (e.key === "Delete" || e.key === "Backspace") {
      e.preventDefault();
      void run("Couldn't remove it", PlaybackService.Remove(index));
    } else if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault();
      const to = index + (e.key === "ArrowUp" ? -1 : 1);
      if (to >= 0 && to < len) void run("Couldn't move it", PlaybackService.Move(index, to));
    }
  };
  return (
    <div
      role="listitem"
      tabIndex={0}
      aria-current={current || undefined}
      aria-label={`${item.name}, ${item.artistName}`}
      draggable={!current && !item.auto}
      onDragStart={(e: DragEvent) => {
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData("text/plain", String(index));
        onDragStart(index);
      }}
      onDragOver={(e) => {
        e.preventDefault();
        onDragOver(index);
      }}
      onDrop={(e) => {
        e.preventDefault();
        onDrop(index);
      }}
      onDoubleClick={() => void run("Couldn't play that song", PlaybackService.Jump(index))}
      onKeyDown={onKeyDown}
      className={cn(
        "group/q relative flex h-[52px] items-center gap-3 rounded-md px-2 transition-colors duration-100 select-none hover:bg-hover focus-visible:bg-hover focus-visible:outline-offset-[-2px]",
        dragOver && "before:absolute before:inset-x-2 before:-top-px before:h-0.5 before:bg-ink",
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
            aria-label={`Remove ${item.name} from the queue`}
            onClick={() => void run("Couldn't remove it", PlaybackService.Remove(index))}
            className="hidden text-ink-muted group-focus-within/q:inline-flex group-hover/q:inline-flex hover:text-ink"
          >
            <X />
          </Button>
        </>
      )}
    </div>
  );
}

/**
 * The queue beside the page: the current song, then what plays next, the
 * user's songs first and then, under their own heading, what the station
 * or AutoPlay adds. Drag or Alt+↑/↓ to reorder, Delete to remove, Enter
 * or double-click to jump.
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
  const [dragFrom, setDragFrom] = useState<number | null>(null);
  const [dragOver, setDragOver] = useState<number | null>(null);

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
      lines.push({
        kind: "head",
        key: "head",
        title: station.autoPlay ? "AutoPlay" : `Then ${station.name}`,
        detail: station.autoPlay
          ? `Similar music, from ${station.name}`
          : "The station continues after your songs",
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
    lines.push({ kind: "note", key: "note", text: "Then AutoPlay continues with similar music" });
  }

  const v = useVirtualizer({
    count: lines.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (i) => (lines[i].kind === "row" ? ROW : HEAD),
    overscan: 8,
  });

  const drop = (to: number) => {
    if (dragFrom !== null && dragFrom !== to) {
      void run("Couldn't move it", PlaybackService.Move(dragFrom, to));
    }
    setDragFrom(null);
    setDragOver(null);
  };

  const summary = !items.length
    ? ""
    : items[start]?.live && !mine
      ? "Live radio"
      : mine
        ? `${plural(mine, "song")} after this one`
        : auto
          ? "Nothing queued after this song"
          : "Nothing after this song";

  return (
    <aside
      aria-label="Queue"
      className="flex h-full w-[320px] shrink-0 flex-col border-l border-line bg-ground"
    >
      <div className="flex h-12 shrink-0 items-center justify-between gap-2 pr-2 pl-4">
        <div className="min-w-0">
          <h2 className="text-heading text-ink">Queue</h2>
        </div>
        <div className="flex items-center gap-1">
          {mine > 0 && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void run("Couldn't clear the queue", PlaybackService.Clear())}
            >
              Clear
            </Button>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="AutoPlay"
                aria-pressed={autoPlay}
                onClick={() => {
                  usePlayback.getState().patch({ autoPlay: !autoPlay });
                  void run("Couldn't change AutoPlay", PlaybackService.SetAutoPlay(!autoPlay));
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
              {autoPlay ? "AutoPlay on: similar music plays when the queue ends" : "AutoPlay off"}
            </TooltipContent>
          </Tooltip>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Close queue"
            onClick={() => close(false)}
          >
            <X strokeWidth={1.75} />
          </Button>
        </div>
      </div>
      {items.length === 0 ? (
        <div className="mx-4 mt-2 rounded-lg border border-dashed border-control-line px-4 py-5">
          <p className="text-ui font-medium text-ink">The queue is empty</p>
          <p className="mt-1 text-sub text-ink-muted">
            Play an album, playlist or station, or choose Play next on any song.
          </p>
        </div>
      ) : (
        <>
          <p className="shrink-0 truncate px-4 pb-2 text-meta text-ink-muted num">
            {station && !station.autoPlay ? (
              <>
                Playing from{" "}
                <Link
                  to="/station/$id"
                  params={{ id: station.id }}
                  className="text-ink hover:underline hover:underline-offset-2"
                >
                  {station.name}
                </Link>
              </>
            ) : (
              summary
            )}
          </p>
          <div
            ref={scrollRef}
            role="list"
            className="min-h-0 flex-1 overflow-y-auto px-2 pb-3"
            onDragLeave={() => setDragOver(null)}
          >
            <div className="relative" style={{ height: v.getTotalSize() }}>
              {v.getVirtualItems().map((row) => {
                const line = lines[row.index];
                const style = { transform: `translateY(${row.start}px)` };
                if (line.kind !== "row") {
                  return (
                    <div
                      key={line.key}
                      role="presentation"
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
                    {qi === start + 1 && <span className="sr-only">Up next</span>}
                    <QueueRow
                      item={item}
                      index={qi}
                      current={qi === index}
                      playing={playing}
                      dragOver={dragOver === qi && dragFrom !== null && dragFrom !== qi}
                      onDragStart={setDragFrom}
                      onDragOver={setDragOver}
                      onDrop={drop}
                    />
                  </div>
                );
              })}
            </div>
          </div>
        </>
      )}
    </aside>
  );
}
