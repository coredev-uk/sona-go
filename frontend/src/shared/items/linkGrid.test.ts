import { describe, expect, it } from "vitest";

import { LINK_GAP, LINK_MIN, linkGrid } from "./linkGrid";

const wide = (cols: number) => cols * LINK_MIN + (cols - 1) * LINK_GAP;

describe("linkGrid", () => {
  it("fits as many columns as hold the minimum width, and fills them downward", () => {
    expect(linkGrid(wide(5), 25)).toEqual({ cols: 5, rows: 5 });
    expect(linkGrid(wide(2), 21)).toEqual({ cols: 2, rows: 11 });
  });

  it("never strands a lone button in the last column", () => {
    // Apple's 21 radio genres: 5+5+5+5+1 would leave Wellbeing alone.
    expect(linkGrid(wide(5), 21)).toEqual({ cols: 5, rows: 6 });
    // 6+6+6+3: the last column holds half a column, which is company enough.
    expect(linkGrid(wide(2), 3)).toEqual({ cols: 2, rows: 2 });
  });

  it("drops a column one pixel short of fitting it", () => {
    expect(linkGrid(wide(3) - 1, 9)).toEqual({ cols: 2, rows: 5 });
    expect(linkGrid(wide(3), 9)).toEqual({ cols: 3, rows: 3 });
  });

  it("fills a short list down by the rows it needs, keeping every track", () => {
    // Apple's eight hubs on a row that fits five: two rows of four, in tracks as wide as the genres'.
    expect(linkGrid(wide(5), 8)).toEqual({ cols: 5, rows: 2 });
    expect(linkGrid(wide(5), 3)).toEqual({ cols: 5, rows: 1 });
  });

  it("is one column before the row has a width, and for an empty list", () => {
    expect(linkGrid(0, 6)).toEqual({ cols: 1, rows: 6 });
    expect(linkGrid(wide(4), 0)).toEqual({ cols: 4, rows: 1 });
  });
});
