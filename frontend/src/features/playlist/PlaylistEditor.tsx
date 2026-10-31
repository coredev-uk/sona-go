import { Trans, useLingui } from "@lingui/react/macro";
import type { Song } from "@bindings/services";
import { GripVertical, X } from "lucide-react";
import { memo, useId, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { beginDrag, dropIndex, useDrag, useDropTarget } from "@/shared/dnd/drag";
import { formatTime } from "@/shared/lib/format";
import { useContentVirtualizer } from "@/shared/lib/useContentVirtualizer";
import { cn } from "@/shared/lib/utils";

const ROW = 44;
/** Lists longer than this are virtualised (docs/architecture.md §8). */
const VIRTUAL_FROM = 200;
const TEMPLATE = "24px 32px minmax(0,2fr) minmax(0,1fr) 56px 32px";

interface PlaylistEditorProps {
  tracks: readonly Song[];
  /** The playlist's name, for accessible labels. */
  name: string;
  onMove: (from: number, to: number) => void;
  onRemove: (index: number) => void;
}

interface RowProps {
  song: Song;
  index: number;
  /** The playlist's name, for the remove button's label. */
  playlist: string;
  focused: boolean;
  dragged: boolean;
  onFocusRow: (index: number) => void;
  onGrab: (e: PointerEvent<HTMLButtonElement>, index: number) => void;
  onRemove: (index: number) => void;
}

/** One editable row; memoised so a drag or focus change re-renders only the rows it touches. */
const EditorRow = memo(function EditorRow({
  song,
  index: i,
  playlist,
  focused,
  dragged,
  onFocusRow,
  onGrab,
  onRemove,
}: RowProps) {
  const { t } = useLingui();
  return (
    <div
      role="row"
      aria-rowindex={i + 1}
      data-row={i}
      tabIndex={focused ? 0 : -1}
      onFocus={(e) => e.target === e.currentTarget && onFocusRow(i)}
      className={cn(
        "group/row grid h-11 items-center gap-3 rounded-md px-1 transition-colors duration-100 select-none hover:bg-hover focus-visible:bg-hover focus-visible:outline-offset-[-2px]",
        dragged && "bg-active opacity-60",
      )}
      style={{ gridTemplateColumns: TEMPLATE }}
    >
      <span role="gridcell" className="flex justify-center">
        <button
          type="button"
          tabIndex={-1}
          aria-label={t`Drag ${{ songName: song.name }} to reorder`}
          onPointerDown={(e) => onGrab(e, i)}
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
          aria-label={t`Remove ${{ songName: song.name }} from ${playlist}`}
          onClick={() => onRemove(i)}
          className="grid size-7 place-items-center rounded-md text-ink-faint transition-colors duration-100 hover:bg-active hover:text-ink"
        >
          <X aria-hidden className="size-4" strokeWidth={1.75} />
        </button>
      </span>
    </div>
  );
});

/**
 * A playlist's tracks in edit mode: drag a row by its handle (or press
 * Alt+↑/↓ on a focused row) to move it; the cross (or Delete) removes it.
 * Rows stay 44px so long playlists virtualise exactly like TrackList.
 */
export function PlaylistEditor({ tracks, name, onMove, onRemove }: PlaylistEditorProps) {
  const { t } = useLingui();
  const bodyRef = useRef<HTMLDivElement>(null);
  const listId = `playlist-editor:${useId()}`;
  const [focus, setFocus] = useState(0);
  const [announce, setAnnounce] = useState("");
  const virtual = tracks.length > VIRTUAL_FROM;
  const {
    virtualizer: v,
    items,
    totalSize,
    margin,
  } = useContentVirtualizer(bodyRef, virtual ? tracks.length : 0, ROW, 12);
  const virtualizer = virtual ? v : undefined;

  const focusRow = (i: number) => {
    const idx = Math.max(0, Math.min(tracks.length - 1, i));
    setFocus(idx);
    virtualizer?.scrollToIndex(idx, { align: "auto" });
    // A virtual row may need a few frames to render after the scroll.
    const go = (tries: number) => {
      const el = bodyRef.current?.querySelector<HTMLElement>(`[data-row="${idx}"]`);
      if (el) el.focus({ preventScroll: !!virtualizer });
      else if (tries < 10) requestAnimationFrame(() => go(tries + 1));
    };
    requestAnimationFrame(() => go(0));
  };

  const move = (from: number, to: number) => {
    if (to < 0 || to >= tracks.length || from === to) return;
    onMove(from, to);
    const songName = tracks[from].name;
    const position = to + 1;
    const count = tracks.length;
    setAnnounce(t`Moved ${songName} to position ${position} of ${count}`);
  };
  const remove = (i: number) => {
    onRemove(i);
    const songName = tracks[i].name;
    setAnnounce(t`Removed ${songName}`);
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

  // Reordering is the shared drag (shared/dnd): the handle starts it, the
  // list is its drop target, and only a change of drop slot re-renders.
  const from = useDrag((s) => (s.active?.origin?.list === listId ? s.active.origin.index : null));
  const [slot, setSlot] = useState<number | null>(null);
  const slotAt = (clientY: number) => {
    const top = bodyRef.current?.getBoundingClientRect().top ?? 0;
    return Math.max(0, Math.min(tracks.length, Math.round((clientY - top) / ROW)));
  };
  useDropTarget(listId, {
    accepts: (p) => p.origin?.list === listId,
    describe: () => t`Move here`,
    hover: (_, at) => {
      const s = at && slotAt(at.y);
      setSlot((prev) => (prev === s ? prev : s));
    },
    drop: (p, at) => {
      const f = p.origin?.index ?? 0;
      const to = dropIndex(f, slotAt(at.y));
      move(f, to);
      setFocus(to);
    },
  });

  const startDrag = (e: PointerEvent<HTMLButtonElement>, i: number) => {
    if (e.button !== 0) return;
    e.preventDefault();
    setFocus(i);
    const song = tracks[i];
    beginDrag(
      e,
      () => ({
        items: [],
        origin: { list: listId, index: i },
        label: song.name,
        art: song.art.small,
      }),
      0,
    );
  };

  const row = (song: Song, i: number) => (
    <EditorRow
      song={song}
      index={i}
      playlist={name}
      focused={i === focus}
      dragged={from === i}
      onFocusRow={setFocus}
      onGrab={startDrag}
      onRemove={remove}
    />
  );

  // The help text and the live region sit beside the grid: a grid may only
  // hold rows.
  return (
    <>
      <p id="playlist-editor-help" className="sr-only">
        <Trans>
          Drag the handle, or press Alt+↑ and Alt+↓ on a song, to change the order. Delete removes
          the focused song.
        </Trans>
      </p>
      <div
        role="grid"
        aria-label={t`Edit the songs in ${name}`}
        aria-rowcount={tracks.length}
        aria-describedby="playlist-editor-help"
        onKeyDown={onKeyDown}
      >
        <div ref={bodyRef} data-drop={listId} className="relative">
          {virtualizer ? (
            <div className="relative" style={{ height: totalSize }}>
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
          {from !== null && slot !== null && slot !== from && slot !== from + 1 && (
            <div
              aria-hidden
              className="pointer-events-none absolute inset-x-1 h-0.5 -translate-y-px rounded-full bg-ink"
              style={{ top: slot * ROW }}
            />
          )}
        </div>
      </div>
      {/* A new paragraph per message: WebKitGTK only tells AT-SPI about added children. */}
      <div aria-live="polite" className="sr-only">
        {announce && <p key={announce}>{announce}</p>}
      </div>
    </>
  );
}
