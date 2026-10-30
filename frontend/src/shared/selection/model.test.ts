import { describe, expect, it } from "vitest";

import {
  clearSelection,
  clickRow,
  emptySelection,
  extendTo,
  menuOn,
  moveAnchor,
  runPosition,
  selectAll,
  selectedIndices,
  type Selection,
} from "./model";

// Row 2 is still loading (no key).
const keys = ["a", "b", undefined, "d", "e", "f"];
const plain = { range: false, toggle: false };
const ctrl = { range: false, toggle: true };
const shift = { range: true, toggle: false };
const ctrlShift = { range: true, toggle: true };

const sel = (s: Selection) => [...s.keys].sort();

describe("selection model", () => {
  it("selects one row on a plain click and moves the anchor there", () => {
    let s = clickRow(emptySelection, keys, 1, plain);
    s = clickRow(s, keys, 4, plain);
    expect(sel(s)).toEqual(["e"]);
    expect(s.anchor).toBe(4);
  });

  it("toggles rows with Ctrl and keeps the rest", () => {
    let s = clickRow(emptySelection, keys, 0, plain);
    s = clickRow(s, keys, 3, ctrl);
    expect(sel(s)).toEqual(["a", "d"]);
    s = clickRow(s, keys, 0, ctrl);
    expect(sel(s)).toEqual(["d"]);
    // A Ctrl-click moves the anchor even when it deselects.
    expect(s.anchor).toBe(0);
  });

  it("selects anchor…row with Shift, pivoting on the same anchor, skipping keyless rows", () => {
    let s = clickRow(emptySelection, keys, 1, plain);
    s = clickRow(s, keys, 4, shift);
    expect(sel(s)).toEqual(["b", "d", "e"]);
    expect(s.anchor).toBe(1);
    // A second Shift-click replaces the range rather than adding to it.
    s = clickRow(s, keys, 0, shift);
    expect(sel(s)).toEqual(["a", "b"]);
    expect(s.anchor).toBe(1);
  });

  it("adds a Ctrl+Shift range to the selection", () => {
    let s = clickRow(emptySelection, keys, 0, plain);
    s = clickRow(s, keys, 4, ctrl);
    s = clickRow(s, keys, 5, ctrlShift);
    expect(sel(s)).toEqual(["a", "e", "f"]);
  });

  it("starts a Shift range at the clicked row when nothing was chosen yet", () => {
    const s = clickRow(emptySelection, keys, 3, shift);
    expect(sel(s)).toEqual(["d"]);
    expect(s.anchor).toBe(3);
  });

  it("ignores clicks on rows that cannot be selected", () => {
    const s = clickRow(emptySelection, keys, 0, plain);
    expect(clickRow(s, keys, 2, plain)).toBe(s);
  });

  it("extends with Shift+arrows from the anchor, and plain arrows move only the anchor", () => {
    let s = clickRow(emptySelection, keys, 3, plain);
    s = extendTo(s, keys, 4);
    s = extendTo(s, keys, 5);
    expect(sel(s)).toEqual(["d", "e", "f"]);
    // Back up past the anchor: the range flips to the other side.
    s = extendTo(s, keys, 1);
    expect(sel(s)).toEqual(["b", "d"]);
    s = moveAnchor(s, 5);
    expect(sel(s)).toEqual(["b", "d"]);
    s = extendTo(s, keys, 4);
    expect(sel(s)).toEqual(["e", "f"]);
  });

  it("selects every selectable row, and clears on Escape keeping the anchor", () => {
    let s = clickRow(emptySelection, keys, 4, plain);
    s = selectAll(s, keys);
    expect(sel(s)).toEqual(["a", "b", "d", "e", "f"]);
    s = clearSelection(s);
    expect(s.keys.size).toBe(0);
    expect(s.anchor).toBe(4);
    expect(clearSelection(s)).toBe(s);
  });

  it("keeps the selection for a menu on a selected row, and replaces it otherwise", () => {
    let s = clickRow(emptySelection, keys, 0, plain);
    s = clickRow(s, keys, 1, ctrl);
    expect(menuOn(s, keys, 1)).toBe(s);
    const other = menuOn(s, keys, 4);
    expect(sel(other)).toEqual(["e"]);
    expect(other.anchor).toBe(4);
  });

  it("follows rows by key when the list re-sorts", () => {
    let s = clickRow(emptySelection, keys, 0, plain);
    s = clickRow(s, keys, 4, ctrl);
    expect(selectedIndices(s, keys)).toEqual([0, 4]);
    expect(selectedIndices(s, ["e", "x", "a"])).toEqual([0, 2]);
  });

  it("draws adjacent selected rows as one run", () => {
    const chosen = new Set(["b", "c", "d", "f"]);
    expect(runPosition(chosen, "a", "b", "c")).toBe("first");
    expect(runPosition(chosen, "b", "c", "d")).toBe("middle");
    expect(runPosition(chosen, "c", "d", "e")).toBe("last");
    expect(runPosition(chosen, "e", "f", undefined)).toBe("single");
    expect(runPosition(chosen, "d", "e", "f")).toBe("none");
    expect(runPosition(chosen, undefined, undefined, "b")).toBe("none");
  });
});
