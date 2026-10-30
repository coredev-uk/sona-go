import { describe, expect, it } from "vitest";

import { dropIndex, edgeScroll } from "./drag";

describe("drag geometry", () => {
  it("drops a dragged row into the gap under the pointer", () => {
    // Dragging row 1 into gap 3 (between rows 2 and 3) lands it at index 2.
    expect(dropIndex(1, 3)).toBe(2);
    expect(dropIndex(3, 0)).toBe(0);
    expect(dropIndex(2, 2)).toBe(2);
  });

  it("scrolls a pane only near its edges, faster the closer the pointer", () => {
    const pane = { top: 100, bottom: 700 };
    expect(edgeScroll(pane, 400)).toBe(0);
    expect(edgeScroll(pane, 156)).toBe(0);
    expect(edgeScroll(pane, 150)).toBeLessThan(0);
    expect(edgeScroll(pane, 100)).toBeLessThan(edgeScroll(pane, 150));
    expect(edgeScroll(pane, 690)).toBeGreaterThan(0);
  });
});
