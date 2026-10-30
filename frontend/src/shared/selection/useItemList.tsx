import {
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
  type RefObject,
} from "react";

import { ContextActionMenu } from "@/shared/actions/ActionMenu";
import { describeItems, type ActionContainer, type ActionItem } from "@/shared/actions/registry";
import { beginDrag, type DragPayload } from "@/shared/dnd/drag";

import {
  clearSelection,
  clickRow,
  extendTo,
  menuOn,
  moveAnchor,
  selectAll,
  type RowKeys,
} from "./model";
import { useSelectionScope, type SelectionScope } from "./store";

interface ItemListOptions {
  /** The list's root: rows inside it carry `data-row={index}`. */
  root: RefObject<HTMLElement | null>;
  /** Row keys in display order (`undefined`: not selectable). */
  keys: RowKeys;
  itemAt: (index: number) => ActionItem | undefined;
  /**
   * "select": a plain click selects the row (track lists, where a
   * double-click plays). "open": a plain click follows the tile's link and
   * only Ctrl/Shift-clicks select (cover grids).
   */
  plainClick: "select" | "open";
  /** The collection the items are listed in, for menu actions. */
  container?: Omit<ActionContainer, "index">;
}

export interface ItemList {
  scope: SelectionScope;
  /** Delegated handlers for the list's root element. */
  rootProps: {
    onClickCapture: (e: MouseEvent) => void;
    onPointerDown: (e: PointerEvent) => void;
    onContextMenu: (e: MouseEvent) => void;
    onDragStart: (e: MouseEvent) => void;
  };
  /**
   * Selection keys for a list's keydown handler, with the focused row:
   * Escape, Ctrl+A, Ctrl+Space, Shift+F10 / the Menu key. True when handled.
   */
  selectionKey: (e: KeyboardEvent, focus: number) => boolean;
  /** After an arrow key moved focus to `index`: Shift extends the selection to it. */
  arrowTo: (e: KeyboardEvent, index: number) => void;
  /** The open context menu, to render inside the list. */
  menu: ReactNode;
}

const rowOf = (target: EventTarget | null) =>
  target instanceof Element ? target.closest<HTMLElement>("[data-row]") : null;

/** The row's focus target: the row itself when focusable, else its opener (a tile's title link). */
function focusTarget(row: HTMLElement | null | undefined): HTMLElement | null {
  if (!row) return null;
  if (row.hasAttribute("tabindex")) return row;
  return row.querySelector<HTMLElement>("[data-opener]");
}

/**
 * Selection, context menu and drag for a list of items, all delegated from
 * the list's root so rows stay plain (memoised, no listeners of their own):
 * click / Ctrl+click / Shift+click select; right-click opens the row's
 * menu at the pointer (acting on the selection when the row is in it);
 * pressing and moving a row drags it, or the selection it belongs to.
 */
export function useItemList({
  root,
  keys,
  itemAt,
  plainClick,
  container,
}: ItemListOptions): ItemList {
  const scope = useSelectionScope(root, { keys, itemAt });
  const { store } = scope;
  const [menu, setMenu] = useState<{
    index: number;
    x: number;
    y: number;
    keyboard: boolean;
  } | null>(null);

  const indexOf = (row: HTMLElement | null) => (row ? Number(row.dataset.row) : -1);

  const openMenu = (index: number, x: number, y: number, keyboard: boolean) => {
    if (!itemAt(index)) return;
    store.setState(menuOn(store.getState(), keys, index));
    setMenu({ index, x, y, keyboard });
  };

  const payload = (index: number): DragPayload | null => {
    const key = keys[index];
    const item = itemAt(index);
    if (key === undefined || !item) return null;
    const group = scope.groupWith(key);
    const items = group ? group.items : [item];
    return {
      items,
      label: items.length === 1 ? item.target.name : describeItems(items),
      art: item.target.art.small || undefined,
    };
  };

  const rootProps: ItemList["rootProps"] = {
    onClickCapture: (e) => {
      const row = rowOf(e.target);
      if (!row || !(e.target instanceof Element)) return;
      const mods = { range: e.shiftKey, toggle: e.ctrlKey || e.metaKey };
      if (mods.range || mods.toggle) {
        if (e.target.closest("button")) return;
        e.preventDefault();
        e.stopPropagation();
      } else if (plainClick === "open" || e.target.closest("a, button, input")) {
        return;
      }
      store.setState(clickRow(store.getState(), keys, indexOf(row), mods));
    },
    onPointerDown: (e) => {
      if (e.button !== 0 || e.ctrlKey || e.shiftKey || e.metaKey) return;
      if (!(e.target instanceof Element) || e.target.closest("button, input")) return;
      const index = indexOf(rowOf(e.target));
      if (index < 0) return;
      beginDrag(e, () => payload(index));
    },
    onContextMenu: (e) => {
      const index = indexOf(rowOf(e.target));
      if (index < 0) return;
      e.preventDefault();
      openMenu(index, e.clientX, e.clientY, false);
    },
    // Images and links would start WebKit's native drag; pointer drags replace it.
    onDragStart: (e) => e.preventDefault(),
  };

  const selectionKey = (e: KeyboardEvent, focus: number) => {
    const s = store.getState();
    if (e.key === "Escape" && s.keys.size > 0) {
      store.setState(clearSelection(s));
    } else if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "a") {
      store.setState(selectAll(s, keys));
    } else if ((e.ctrlKey || e.metaKey) && e.key === " ") {
      store.setState(clickRow(s, keys, focus, { range: false, toggle: true }));
    } else if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) {
      const row = root.current?.querySelector<HTMLElement>(`[data-row="${focus}"]`);
      const r = row?.getBoundingClientRect();
      if (!r) return false;
      openMenu(focus, r.left + Math.min(48, r.width / 2), r.top + Math.min(r.height, 40), true);
    } else {
      return false;
    }
    e.preventDefault();
    e.stopPropagation();
    return true;
  };

  const arrowTo = (e: KeyboardEvent, index: number) => {
    const s = store.getState();
    store.setState(e.shiftKey ? extendTo(s, keys, index) : moveAnchor(s, index));
  };

  const item = menu ? itemAt(menu.index) : undefined;
  const menuNode =
    menu && item ? (
      <ContextActionMenu
        key={`${menu.index}:${menu.x}:${menu.y}`}
        item={item}
        name={item.target.name}
        at={menu}
        container={container && { ...container, index: menu.index }}
        selectionKey={keys[menu.index]}
        keyboard={menu.keyboard}
        onClose={() => setMenu(null)}
        returnFocus={() =>
          focusTarget(root.current?.querySelector<HTMLElement>(`[data-row="${menu.index}"]`))
        }
      />
    ) : null;

  return { scope, rootProps, selectionKey, arrowTo, menu: menuNode };
}
