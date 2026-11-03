import { describe, expect, it } from "vitest";

import { moveFrom } from "./roving";

describe("moveFrom", () => {
  // Three rows: a link and tickets, a link alone (no seller), a link and tickets.
  const counts = [2, 1, 2];
  it("moves between rows keeping the column, clamped to a short row", () => {
    expect(moveFrom("ArrowDown", { row: 0, col: 1 }, counts)).toEqual({ row: 1, col: 0 });
    expect(moveFrom("ArrowDown", { row: 1, col: 0 }, counts)).toEqual({ row: 2, col: 0 });
    expect(moveFrom("ArrowUp", { row: 2, col: 1 }, counts)).toEqual({ row: 1, col: 0 });
  });
  it("stops at the ends of the list", () => {
    expect(moveFrom("ArrowUp", { row: 0, col: 0 }, counts)).toEqual({ row: 0, col: 0 });
    expect(moveFrom("ArrowDown", { row: 2, col: 1 }, counts)).toEqual({ row: 2, col: 1 });
  });
  it("moves along a row", () => {
    expect(moveFrom("ArrowRight", { row: 0, col: 0 }, counts)).toEqual({ row: 0, col: 1 });
    expect(moveFrom("ArrowRight", { row: 0, col: 1 }, counts)).toEqual({ row: 0, col: 1 });
    expect(moveFrom("ArrowLeft", { row: 2, col: 1 }, counts)).toEqual({ row: 2, col: 0 });
    expect(moveFrom("ArrowRight", { row: 1, col: 0 }, counts)).toEqual({ row: 1, col: 0 });
  });
  it("goes to the first and last row", () => {
    expect(moveFrom("Home", { row: 2, col: 1 }, counts)).toEqual({ row: 0, col: 1 });
    expect(moveFrom("End", { row: 0, col: 1 }, counts)).toEqual({ row: 2, col: 1 });
    expect(moveFrom("End", { row: 0, col: 1 }, [2, 1])).toEqual({ row: 1, col: 0 });
  });
  it("leaves other keys alone", () => {
    expect(moveFrom("Enter", { row: 0, col: 0 }, counts)).toBeNull();
    expect(moveFrom("a", { row: 0, col: 0 }, counts)).toBeNull();
  });
});
