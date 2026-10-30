import type { Song } from "@bindings/services";
import { GripVertical, X } from "lucide-react";
import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { formatTime } from "@/shared/lib/format";
import { useContentVirtualizer } from "@/shared/lib/useContentVirtualizer";
import { cn } from "@/shared/lib/utils";

import { dropIndex } from "./edit";

const ROW = 44;
/** Lists longer than this are virtualised. */
const VIRTUAL_FROM = 200;
/** Distance from the scroll pane's edge where a drag scrolls it. */
const EDGE = 56;
const TEMPLATE = "24px 32px minmax(0,2fr) minmax(0,1fr) 56px 32px";

interface PlaylistEditorProps {
  tracks: readonly Song[];
  /** The playlist's name, for accessible labels. */
  name: string;
  onMove: (from: number, to: number) => void;
  onRemove: (index: number) => void;
}

interface Drag {
  from: number;
  /** The gap the track would drop into, 0…n. */
  slot: number;
  y: number;
}

/**
 * A playlist's tracks in edit mode: drag a row by its handle (or press
 * Alt+↑/↓ on a focused row) to move it; the cross (or Delete) removes it.
 * Rows stay 44px so long playlists virtualise exactly like TrackList.
 */
export function PlaylistEditor({ tracks, name, onMove, onRemove }: PlaylistEditorProps) {
  const bodyRef = useRef<HTMLDivElement>(null);
  const [focus, setFocus] = useState(0);
  const [drag, setDrag] = useState<Drag | null>(null);
  const [announce, setAnnounce] = useState("");
  const virtual = tracks.length > VIRTUAL_FROM;
  const { virtualizer: v, margin } = useContentVirtualizer(
    bodyRef,
    virtual ? tracks.length : 0,
    ROW,
    12,
  );
  const virtualizer = virtual ? v : undefined;

  const focusRow = (i: number) => {
    const idx = Math.max(0, Math.min(tracks.length - 1, i));
    setFocus(idx);
    virtualizer?.scrollToIndex(idx, { align: "auto" });
    let tries = 0;
    const go = () => {
      const el = bodyRef.current?.querySelector<HTMLElement>(`[data-row="${idx}"]`);
      if (el) el.focus({ preventScroll: !!virtualizer });
      else if (tries++ < 10) requestAnimationFrame(go);
    };
    requestAnimationFrame(go);
  };

  const move = (from: number, to: number) => {
    if (to < 0 || to >= tracks.length || from === to) return;
    onMove(from, to);
    setAnnounce(`Moved ${tracks[from].name} to position ${to + 1} of ${tracks.length}`);
  };
  const remove = (i: number) => {
    onRemove(i);
    setAnnounce(`Removed ${tracks[i].name}`);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.target instanceof HTMLElement && e.target.getAttribute("role") !== "row") return;
    const page = Math.max(1, Math.floor(window.innerHeight / ROW) - 2);
    if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      const to = focus + (e.key === "ArrowUp" ? -1 : 1);
      move(focus, to);
      focusRow(to);
    } else if (e.key === "ArrowDown") focusRow(focus + 1);
    else if (e.key === "ArrowUp") focusRow(focus - 1);
    else if (e.key === "PageDown") focusRow(focus + page);
    else if (e.key === "PageUp") focusRow(focus - page);
    else if (e.key === "Home") focusRow(0);
    else if (e.key === "End") focusRow(tracks.length - 1);
    else if (e.key === "Delete" || e.key === "Backspace") {
      remove(focus);
      focusRow(Math.min(focus, tracks.length - 2));
    } else return;
    e.preventDefault();
  };

  // The drag lives in a ref (read by the autoscroll loop) mirrored to state
  // (what renders).
  const dragRef = useRef<Drag | null>(null);
  const update = (d: Drag | null) => {
    dragRef.current = d;
    setDrag(d);
  };
  const slotAt = (clientY: number) => {
    const top = bodyRef.current?.getBoundingClientRect().top ?? 0;
    return Math.max(0, Math.min(tracks.length, Math.round((clientY - top) / ROW)));
  };
  const slotRef = useRef(slotAt);
  useEffect(() => {
    slotRef.current = slotAt;
  });

  // While dragging, the pane scrolls when the pointer rests near its edge.
  const dragging = drag !== null;
  useEffect(() => {
    if (!dragging) return;
    const pane = document.getElementById("content");
    let frame = 0;
    const tick = () => {
      const d = dragRef.current;
      if (pane && d) {
        const r = pane.getBoundingClientRect();
        const over =
          d.y < r.top + EDGE
            ? d.y - (r.top + EDGE)
            : d.y > r.bottom - EDGE
              ? d.y - (r.bottom - EDGE)
              : 0;
        if (over) {
          pane.scrollTop += Math.sign(over) * Math.ceil(Math.abs(over) / 4);
          const next = { ...d, slot: slotRef.current(d.y) };
          dragRef.current = next;
          setDrag(next);
        }
      }
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [dragging]);

  const startDrag = (e: PointerEvent<HTMLButtonElement>, i: number) => {
    if (e.button !== 0) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    setFocus(i);
    update({ from: i, slot: i, y: e.clientY });
  };
  const moveDrag = (e: PointerEvent<HTMLButtonElement>) => {
    const d = dragRef.current;
    if (d) update({ ...d, slot: slotAt(e.clientY), y: e.clientY });
  };
  const endDrag = () => {
    const d = dragRef.current;
    if (!d) return;
    const to = dropIndex(d.from, d.slot);
    update(null);
    move(d.from, to);
    setFocus(to);
  };

  const row = (song: Song, i: number) => (
    <div
      role="row"
      aria-rowindex={i + 1}
      data-row={i}
      tabIndex={i === focus ? 0 : -1}
      onFocus={(e) => e.target === e.currentTarget && setFocus(i)}
      className={cn(
        "group/row grid h-11 items-center gap-3 rounded-md px-1 transition-colors duration-100 select-none hover:bg-hover focus-visible:bg-hover focus-visible:outline-offset-[-2px]",
        drag?.from === i && "bg-active opacity-60",
      )}
      style={{ gridTemplateColumns: TEMPLATE }}
    >
      <span role="gridcell" className="flex justify-center">
        <button
          type="button"
          tabIndex={-1}
          aria-label={`Drag ${song.name} to reorder`}
          onPointerDown={(e) => startDrag(e, i)}
          onPointerMove={moveDrag}
          onPointerUp={endDrag}
          onPointerCancel={() => update(null)}
          className="grid h-8 w-6 cursor-grab touch-none place-items-center rounded-sm text-ink-faint hover:text-ink active:cursor-grabbing"
        >
          <GripVertical aria-hidden className="size-4" strokeWidth={1.75} />
        </button>
      </span>
      <span role="gridcell" className="text-right text-sub text-ink-faint num">
        {i + 1}
      </span>
      <span role="gridcell" className="flex min-w-0 items-center gap-3">
        <Artwork src={song.art.small} size={32} alt="" />
        <span className="min-w-0">
          <span className="block truncate text-ui text-ink">{song.name}</span>
          <span className="block truncate text-sub text-ink-muted">{song.artistName}</span>
        </span>
      </span>
      <span role="gridcell" className="truncate text-sub text-ink-muted">
        {song.albumName}
      </span>
      <span role="gridcell" className="text-right text-sub text-ink-faint num">
        {song.durationMs ? formatTime(song.durationMs) : "—"}
      </span>
      <span role="gridcell" className="flex justify-end">
        <button
          type="button"
          tabIndex={-1}
          aria-label={`Remove ${song.name} from ${name}`}
          onClick={() => remove(i)}
          className="grid size-7 place-items-center rounded-md text-ink-faint transition-colors duration-100 hover:bg-active hover:text-ink"
        >
          <X aria-hidden className="size-4" strokeWidth={1.75} />
        </button>
      </span>
    </div>
  );

  const items = virtualizer?.getVirtualItems() ?? [];
  return (
    <div
      role="grid"
      aria-label={`Edit the songs in ${name}`}
      aria-rowcount={tracks.length}
      aria-describedby="playlist-editor-help"
      onKeyDown={onKeyDown}
    >
      <p id="playlist-editor-help" className="sr-only">
        Drag the handle, or press Alt+↑ and Alt+↓ on a song, to change the order. Delete removes the
        focused song.
      </p>
      <div ref={bodyRef} className="relative">
        {virtualizer ? (
          <div className="relative" style={{ height: virtualizer.getTotalSize() }}>
            {items.map((it) => (
              <div
                key={it.key}
                className="absolute inset-x-0 top-0"
                style={{ transform: `translateY(${it.start - margin}px)` }}
              >
                {row(tracks[it.index], it.index)}
              </div>
            ))}
          </div>
        ) : (
          tracks.map((song, i) => <div key={`${song.id}-${i}`}>{row(song, i)}</div>)
        )}
        {drag && drag.slot !== drag.from && drag.slot !== drag.from + 1 && (
          <div
            aria-hidden
            className="pointer-events-none absolute inset-x-1 h-0.5 -translate-y-px rounded-full bg-ink"
            style={{ top: drag.slot * ROW }}
          />
        )}
      </div>
      <div aria-live="polite" className="sr-only">
        {announce}
      </div>
    </div>
  );
}
