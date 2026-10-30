import { createContext, useContext, useEffect, useState, type RefObject } from "react";
import { createStore, useStore, type StoreApi } from "zustand";

import type { ActionItem } from "@/shared/actions/registry";

import {
  emptySelection,
  runPosition,
  selectedIndices,
  type RowKeys,
  type Selection,
} from "./model";

/**
 * One list's selection, shared with its rows and menus through context.
 * Rows subscribe with a selector (`useRowSelected`, `useRowRun`), so a
 * change re-renders only the rows whose own state changed; the list itself
 * does not re-render on selection changes.
 */
export interface SelectionScope {
  store: StoreApi<Selection>;
  /** The selected rows' items in list order, with their positions. */
  selected(): SelectedItems;
  /** The selection as menus and drags use it: two or more rows including `key`, else null. */
  groupWith(key: string): SelectedItems | null;
  /** Points the scope at the list's rows as last rendered. */
  setRows(rows: ScopeRows): void;
}

export interface SelectedItems {
  items: ActionItem[];
  indices: number[];
}

/** The rows a scope reads when asked (a list's rows as last rendered). */
export interface ScopeRows {
  keys: RowKeys;
  itemAt: (index: number) => ActionItem | undefined;
}

export const SelectionContext = createContext<SelectionScope | null>(null);

const noSelection = createStore<Selection>(() => emptySelection);

/** A selection scope over `initial` rows; `useSelectionScope` makes one per list. */
export function createSelectionScope(initial: ScopeRows): SelectionScope {
  const store = createStore<Selection>(() => emptySelection);
  let rows = initial;
  const selected = (): SelectedItems => {
    const items: ActionItem[] = [];
    const indices: number[] = [];
    for (const i of selectedIndices(store.getState(), rows.keys)) {
      const item = rows.itemAt(i);
      if (item) {
        items.push(item);
        indices.push(i);
      }
    }
    return { items, indices };
  };
  return {
    store,
    selected,
    groupWith: (key) => {
      const s = store.getState();
      return s.keys.size > 1 && s.keys.has(key) ? selected() : null;
    },
    setRows: (next) => {
      rows = next;
    },
  };
}

/**
 * A list's selection scope over its rows as rendered. While anything is
 * selected, a press outside `root` (and outside menus, dialogs and toasts)
 * clears the selection.
 */
export function useSelectionScope(
  root: RefObject<HTMLElement | null>,
  rows: ScopeRows,
): SelectionScope {
  const [scope] = useState(() => createSelectionScope(rows));
  useEffect(() => scope.setRows(rows));

  const { store } = scope;
  useEffect(() => {
    let attached = false;
    const onDown = (e: PointerEvent) => {
      const t = e.target instanceof Element ? e.target : null;
      if (!t || root.current?.contains(t)) return;
      if (t.closest("[role=menu], [role=dialog], [role=alertdialog], [data-sonner-toaster]"))
        return;
      store.setState({ keys: new Set(), anchor: null });
    };
    const sync = (s: Selection) => {
      const want = s.keys.size > 0;
      if (want === attached) return;
      attached = want;
      if (want) document.addEventListener("pointerdown", onDown, true);
      else document.removeEventListener("pointerdown", onDown, true);
    };
    const unsub = store.subscribe(sync);
    return () => {
      unsub();
      if (attached) document.removeEventListener("pointerdown", onDown, true);
    };
  }, [store, root]);

  return scope;
}

/** Whether row `key` is selected in the enclosing list (false outside one). */
export function useRowSelected(key: string | undefined): boolean {
  const scope = useContext(SelectionContext);
  return useStore(scope?.store ?? noSelection, (s) => key !== undefined && s.keys.has(key));
}

/** Where row `key` sits in its run of selected neighbours (see runPosition). */
export function useRowRun(
  prev: string | undefined,
  key: string | undefined,
  next: string | undefined,
) {
  const scope = useContext(SelectionContext);
  return useStore(scope?.store ?? noSelection, (s) => runPosition(s.keys, prev, key, next));
}
