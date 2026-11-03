import { describe, expect, it } from "vitest";

import { focusedRow, menuMoves, moveForKey, rowForKey } from "./queue-focus";

const q = (...uids: number[]) => uids.map((uid) => ({ uid }));
const uids = q(10, 11, 12, 13, 14);

describe("focusedRow", () => {
  it("starts on the current song", () => {
    expect(focusedRow(uids, 1, null)).toBe(1);
  });

  it("follows the entry when it moves", () => {
    expect(focusedRow(q(10, 12, 11, 13, 14), 0, { uid: 11, qi: 1 })).toBe(2);
  });

  it("lands on the next row when the focused one is removed, the last one when it was last", () => {
    expect(focusedRow(q(10, 11, 13, 14), 0, { uid: 12, qi: 2 })).toBe(2);
    expect(focusedRow(q(10, 11, 12, 13), 0, { uid: 14, qi: 4 })).toBe(3);
  });

  it("moves to the current song when the focused one has been played", () => {
    expect(focusedRow(uids, 3, { uid: 11, qi: 1 })).toBe(3);
  });

  it("has no row when nothing is listed", () => {
    expect(focusedRow([], 0, null)).toBe(-1);
    expect(focusedRow(uids, 5, { uid: 14, qi: 4 })).toBe(-1);
  });
});

describe("rowForKey", () => {
  it("steps and jumps within the listed rows", () => {
    expect(rowForKey("ArrowDown", 2, 1, 5)).toBe(3);
    expect(rowForKey("ArrowUp", 2, 1, 5)).toBe(1);
    expect(rowForKey("Home", 3, 1, 5)).toBe(1);
    expect(rowForKey("End", 1, 1, 5)).toBe(4);
  });

  it("stops at the ends instead of wrapping or reaching played songs", () => {
    expect(rowForKey("ArrowUp", 1, 1, 5)).toBe(1);
    expect(rowForKey("ArrowDown", 4, 1, 5)).toBe(4);
  });

  it("ignores other keys", () => {
    expect(rowForKey("Enter", 2, 1, 5)).toBeNull();
  });
});

describe("moveForKey", () => {
  it("moves one step either way", () => {
    expect(moveForKey("ArrowUp", 2, 5)).toBe(1);
    expect(moveForKey("ArrowDown", 2, 5)).toBe(3);
  });

  it("keeps the user's songs before AutoPlay and inside the queue", () => {
    expect(moveForKey("ArrowDown", 4, 5)).toBeNull();
    expect(moveForKey("ArrowUp", 0, 5)).toBeNull();
  });

  it("ignores other keys", () => {
    expect(moveForKey("Home", 2, 5)).toBeNull();
  });
});

describe("menuMoves", () => {
  // Playing queue[1]; the user's songs are queue[2..4], AutoPlay's start at 5.
  it("offers both for a song in the middle of what plays next", () => {
    expect(menuMoves(3, 1, 5, false)).toEqual({ next: true, end: true });
  });

  it("leaves out the move that would change nothing", () => {
    expect(menuMoves(2, 1, 5, false)).toEqual({ next: false, end: true });
    expect(menuMoves(4, 1, 5, false)).toEqual({ next: true, end: false });
    expect(menuMoves(2, 1, 3, false)).toEqual({ next: false, end: false });
  });

  it("offers neither for the playing song or for station and AutoPlay songs", () => {
    expect(menuMoves(1, 1, 5, false)).toEqual({ next: false, end: false });
    expect(menuMoves(6, 1, 5, true)).toEqual({ next: false, end: false });
  });

  it("counts from the first song when nothing is playing", () => {
    expect(menuMoves(0, -1, 3, false)).toEqual({ next: false, end: true });
    expect(menuMoves(1, -1, 3, false)).toEqual({ next: true, end: true });
  });
});
