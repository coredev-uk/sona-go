import { useEffect, useRef } from "react";
import { create } from "zustand";

import type { ActionItem } from "@/shared/actions/registry";

/**
 * Sona's one drag-and-drop mechanism: pointer events, not HTML5 DnD.
 * WebKitGTK's native drag runs a GTK drag session (Wails also listens to
 * it for file drops): `dataTransfer` is unreadable during dragover, the
 * drag image is the whole row snapshot, and a drop outside the page can
 * end in a stuck session. Pointer drags have none of that, work the same
 * in every list, and let targets show what a drop will do.
 *
 * A source calls `beginDrag` from a pointerdown (lists delegate it from
 * their container, so rows carry no listeners). The drag starts once the
 * pointer travels a few pixels; until then a click stays a click. Targets
 * are elements carrying `data-drop={id}`, registered with `useDropTarget`;
 * the innermost one under the pointer that accepts the payload is "over".
 * The scroll pane under the pointer scrolls when the pointer nears its edge.
 */

/** What is being dragged. */
export interface DragPayload {
  /** The items, in order (empty for a queue row, which only reorders). */
  items: readonly ActionItem[];
  /** A reorder inside one list: the list's id and the dragged row's position. */
  origin?: { list: string; index: number };
  /** The drag image's title ("Airbag", "12 songs") and artwork URL. */
  label: string;
  art?: string;
}

export interface Point {
  x: number;
  y: number;
}

export interface DropTarget {
  accepts(p: DragPayload): boolean;
  /** What a drop does, under the drag image's title: "Add to “Road trip”". */
  describe(p: DragPayload): string;
  /** The pointer moved over the target (positional targets); `null` when it left. */
  hover?(p: DragPayload, at: Point | null): void;
  drop(p: DragPayload, at: Point): void;
}

interface DragState {
  /** The payload of the drag in progress. */
  active: DragPayload | null;
  /** The id of the accepting target under the pointer. */
  over: string | null;
}

export const useDrag = create<DragState>(() => ({ active: null, over: null }));

const targets = new Map<string, DropTarget>();

/**
 * Makes the element carrying the returned `data-drop` attribute a drop
 * target while mounted. Returns whether a drag is over it (only this
 * component re-renders when that changes).
 */
export function useDropTarget(id: string, target: DropTarget): boolean {
  const ref = useRef(target);
  useEffect(() => {
    ref.current = target;
  });
  useEffect(() => {
    const t: DropTarget = {
      accepts: (p) => ref.current.accepts(p),
      describe: (p) => ref.current.describe(p),
      hover: (p, at) => ref.current.hover?.(p, at),
      drop: (p, at) => ref.current.drop(p, at),
    };
    targets.set(id, t);
    return () => {
      if (targets.get(id) === t) targets.delete(id);
    };
  }, [id]);
  return useDrag((s) => s.over === id);
}

/** Whether a drag that `accepts` would take is in progress (targets that appear only then). */
export function useDragActive(accepts: (p: DragPayload) => boolean): boolean {
  return useDrag((s) => !!s.active && accepts(s.active));
}

/** Pointer travel before a press becomes a drag. */
const THRESHOLD = 6;
/** Distance from a scroll pane's edge where a drag scrolls it. */
const EDGE = 56;

const scrollable = new WeakMap<Element, boolean>();

function scrollPaneOf(el: Element | null): HTMLElement | null {
  for (let e = el; e && e !== document.body; e = e.parentElement) {
    let s = scrollable.get(e);
    if (s === undefined) {
      const o = getComputedStyle(e).overflowY;
      s = o === "auto" || o === "scroll";
      scrollable.set(e, s);
    }
    if (s && e.scrollHeight > e.clientHeight) return e as HTMLElement;
  }
  return null;
}

/** How far to scroll a pane this frame for a pointer at `y` (0 away from its edges). */
export function edgeScroll(rect: { top: number; bottom: number }, y: number): number {
  const over =
    y < rect.top + EDGE
      ? y - (rect.top + EDGE)
      : y > rect.bottom - EDGE
        ? y - (rect.bottom - EDGE)
        : 0;
  return over && Math.sign(over) * Math.ceil(Math.abs(over) / 4);
}

/** Where a row dragged from `from` lands when dropped into gap `slot` (0…n) of its list. */
export function dropIndex(from: number, slot: number): number {
  return slot > from ? slot - 1 : slot;
}

/** The drag image: the payload's art and title, and what a drop here would do. */
interface Ghost {
  move(x: number, y: number): void;
  caption(s: string): void;
  remove(): void;
}

function ghost(p: DragPayload): Ghost {
  const el = document.createElement("div");
  el.setAttribute("aria-hidden", "true");
  el.className =
    "pointer-events-none fixed top-0 left-0 z-[100] flex max-w-72 items-center gap-2.5 rounded-lg border border-line bg-raised py-1.5 pr-3 pl-1.5 shadow-float";
  if (p.art) {
    const img = document.createElement("img");
    img.src = p.art;
    img.alt = "";
    img.width = 32;
    img.height = 32;
    img.className = "size-8 shrink-0 rounded-sm";
    el.append(img);
  }
  const text = document.createElement("div");
  text.className = "min-w-0";
  const title = document.createElement("p");
  title.className = "truncate text-ui text-ink";
  title.textContent = p.label;
  const caption = document.createElement("p");
  caption.className = "truncate text-meta text-ink-muted";
  text.append(title, caption);
  el.append(text);
  document.body.append(el);
  return {
    move: (x: number, y: number) => {
      // Beside the pointer, flipped to its left near the window's right edge.
      const w = el.offsetWidth;
      const left = x + 14 + w > window.innerWidth ? x - 14 - w : x + 14;
      el.style.transform = `translate(${left}px,${y + 10}px)`;
    },
    caption: (s: string) => {
      caption.textContent = s;
      caption.hidden = !s;
    },
    remove: () => el.remove(),
  };
}

let busy = false;

/**
 * Starts tracking a press that may become a drag. `payload` is read when
 * the pointer has moved far enough (null: not draggable). `threshold` 0
 * starts at once (a drag handle).
 */
export function beginDrag(
  e: { button: number; clientX: number; clientY: number },
  payload: () => DragPayload | null,
  threshold = THRESHOLD,
): void {
  if (e.button !== 0 || busy) return;
  busy = true;
  const sx = e.clientX;
  const sy = e.clientY;
  let x = sx;
  let y = sy;
  let p: DragPayload | null = null;
  let g: Ghost | null = null;
  let over: string | null = null;
  let pane: HTMLElement | null = null;
  let frame = 0;

  const hitTest = () => {
    if (!p || !g) return;
    g.move(x, y);
    const hit = document.elementFromPoint(x, y);
    pane = scrollPaneOf(hit) ?? pane;
    const el = hit?.closest<HTMLElement>("[data-drop]");
    let id = el?.dataset.drop ?? null;
    let t = id ? targets.get(id) : undefined;
    if (!t?.accepts(p)) {
      // An inner target that declines lets an outer one take the drop.
      const outer = el?.parentElement?.closest<HTMLElement>("[data-drop]");
      id = outer?.dataset.drop ?? null;
      t = id ? targets.get(id) : undefined;
      if (!t?.accepts(p)) {
        id = null;
        t = undefined;
      }
    }
    if (id !== over) {
      if (over) targets.get(over)?.hover?.(p, null);
      over = id;
      useDrag.setState({ over });
      g.caption(t ? t.describe(p) : "");
    }
    t?.hover?.(p, { x, y });
  };

  const tick = () => {
    if (pane) {
      const r = pane.getBoundingClientRect();
      const dy = x >= r.left && x <= r.right ? edgeScroll(r, y) : 0;
      if (dy) {
        const before = pane.scrollTop;
        pane.scrollTop += dy;
        if (pane.scrollTop !== before) hitTest();
      }
    }
    frame = requestAnimationFrame(tick);
  };

  const noSelect = (ev: Event) => ev.preventDefault();

  const start = (): boolean => {
    p = payload();
    if (!p) return false;
    g = ghost(p);
    g.caption("");
    g.move(x, y);
    document.getSelection()?.removeAllRanges();
    document.addEventListener("selectstart", noSelect, true);
    document.body.style.userSelect = "none";
    document.body.style.cursor = "grabbing";
    useDrag.setState({ active: p, over: null });
    frame = requestAnimationFrame(tick);
    return true;
  };

  const end = () => {
    busy = false;
    window.removeEventListener("pointermove", onMove, true);
    window.removeEventListener("pointerup", onUp, true);
    window.removeEventListener("pointercancel", end, true);
    window.removeEventListener("keydown", onKey, true);
    cancelAnimationFrame(frame);
    if (p && over) targets.get(over)?.hover?.(p, null);
    g?.remove();
    if (p) {
      document.removeEventListener("selectstart", noSelect, true);
      document.body.style.userSelect = "";
      document.body.style.cursor = "";
      useDrag.setState({ active: null, over: null });
    }
  };

  // A press that became a drag ends without a click (a tile's link must not open).
  const swallowClick = () => {
    const swallow = (ev: Event) => {
      ev.preventDefault();
      ev.stopPropagation();
    };
    window.addEventListener("click", swallow, { capture: true, once: true });
    setTimeout(() => window.removeEventListener("click", swallow, true), 0);
  };

  const onMove = (ev: PointerEvent) => {
    x = ev.clientX;
    y = ev.clientY;
    if (!p) {
      if (Math.hypot(x - sx, y - sy) < threshold) return;
      if (!start()) return end();
    }
    ev.preventDefault();
    hitTest();
  };

  const onUp = () => {
    const dropped = p;
    const target = over ? targets.get(over) : undefined;
    end();
    if (!dropped) return;
    swallowClick();
    target?.drop(dropped, { x, y });
  };

  const onKey = (ev: KeyboardEvent) => {
    if (ev.key !== "Escape" || !p) return;
    ev.preventDefault();
    ev.stopPropagation();
    end();
    // The button is still down: its release must not click what is under it.
    window.addEventListener("pointerup", swallowClick, { capture: true, once: true });
  };

  window.addEventListener("pointermove", onMove, true);
  window.addEventListener("pointerup", onUp, true);
  window.addEventListener("pointercancel", end, true);
  window.addEventListener("keydown", onKey, true);
  if (threshold === 0 && !start()) end();
}
