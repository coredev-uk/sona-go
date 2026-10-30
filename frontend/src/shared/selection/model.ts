/**
 * The multi-select model shared by track lists and cover grids: which rows
 * are selected (by key, so a selection survives re-sorting and paged
 * loading) and the anchor that Shift ranges start from (a position in the
 * list as shown). Rows without a key (still loading, not selectable) are
 * never selected. Pure functions; `store.ts` holds one per list.
 */
export interface Selection {
  readonly keys: ReadonlySet<string>;
  /** Where Shift+click and Shift+arrow ranges start; null before any choice. */
  readonly anchor: number | null;
}

export const emptySelection: Selection = { keys: new Set(), anchor: null };

/** The list's row keys in display order; `undefined` for rows that cannot be selected. */
export type RowKeys = readonly (string | undefined)[];

export interface ClickMods {
  /** Shift: select the range from the anchor. */
  range: boolean;
  /** Ctrl (or Cmd): toggle one row, or add a range to what is selected. */
  toggle: boolean;
}

function rangeKeys(keys: RowKeys, a: number, b: number): string[] {
  const out: string[] = [];
  for (let i = Math.max(0, Math.min(a, b)); i <= Math.min(keys.length - 1, Math.max(a, b)); i++) {
    const k = keys[i];
    if (k !== undefined) out.push(k);
  }
  return out;
}

/**
 * A click on row `index`: alone it selects just that row; Ctrl toggles it;
 * Shift selects anchor…index (Ctrl+Shift adds that range). The anchor moves
 * to a clicked row except on Shift, so successive Shift-clicks pivot on it.
 */
export function clickRow(sel: Selection, keys: RowKeys, index: number, mods: ClickMods): Selection {
  const key = keys[index];
  if (key === undefined) return sel;
  if (mods.range) {
    const anchor = sel.anchor ?? index;
    const next = new Set(mods.toggle ? sel.keys : []);
    for (const k of rangeKeys(keys, anchor, index)) next.add(k);
    return { keys: next, anchor };
  }
  if (mods.toggle) {
    const next = new Set(sel.keys);
    if (!next.delete(key)) next.add(key);
    return { keys: next, anchor: index };
  }
  return { keys: new Set([key]), anchor: index };
}

/** Shift+arrow onto row `index`: the selection becomes anchor…index. */
export function extendTo(sel: Selection, keys: RowKeys, index: number): Selection {
  const anchor = sel.anchor ?? index;
  return { keys: new Set(rangeKeys(keys, anchor, index)), anchor };
}

/** A plain arrow onto row `index`: the selection stays, the next range starts there. */
export function moveAnchor(sel: Selection, index: number): Selection {
  return sel.anchor === index ? sel : { keys: sel.keys, anchor: index };
}

/** Ctrl+A: every selectable row. */
export function selectAll(sel: Selection, keys: RowKeys): Selection {
  const next = new Set<string>();
  for (const k of keys) if (k !== undefined) next.add(k);
  return { keys: next, anchor: sel.anchor };
}

/** Right-click (or the menu key) on a row outside the selection selects just it; inside, keeps it. */
export function menuOn(sel: Selection, keys: RowKeys, index: number): Selection {
  const key = keys[index];
  if (key === undefined || sel.keys.has(key)) return sel;
  return { keys: new Set([key]), anchor: index };
}

/** Escape. */
export function clearSelection(sel: Selection): Selection {
  return sel.keys.size === 0 ? sel : { keys: new Set(), anchor: sel.anchor };
}

/** The positions of the selected rows, in list order. */
export function selectedIndices(sel: Selection, keys: RowKeys): number[] {
  const out: number[] = [];
  if (sel.keys.size === 0) return out;
  keys.forEach((k, i) => {
    if (k !== undefined && sel.keys.has(k)) out.push(i);
  });
  return out;
}

/**
 * Where a selected row sits in its run of adjacent selected rows, so a run
 * is drawn as one block: "none" when it is not selected.
 */
export type RunPosition = "none" | "single" | "first" | "middle" | "last";

export function runPosition(
  keys: ReadonlySet<string>,
  prev: string | undefined,
  key: string | undefined,
  next: string | undefined,
): RunPosition {
  if (key === undefined || !keys.has(key)) return "none";
  const before = prev !== undefined && keys.has(prev);
  const after = next !== undefined && keys.has(next);
  if (before && after) return "middle";
  if (before) return "last";
  if (after) return "first";
  return "single";
}
