import type { PlaylistDetail, Song } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { editing, serial, withMove, withRemoved } from "./edit";

const detail = (ids: string[]): PlaylistDetail =>
  ({
    playlist: { id: "p.x", trackCount: ids.length },
    tracks: ids.map((id) => ({ id, durationMs: 1000 }) as Song),
    durationMs: ids.length * 1000,
  }) as PlaylistDetail;

const ids = (d: PlaylistDetail) => d.tracks.map((t) => t.id);

describe("optimistic edits", () => {
  it("moves a track", () => {
    expect(ids(withMove(detail(["a", "b", "c", "d"]), 0, 2))).toEqual(["b", "c", "a", "d"]);
    expect(ids(withMove(detail(["a", "b", "c", "d"]), 3, 0))).toEqual(["d", "a", "b", "c"]);
    const d = detail(["a", "b"]);
    expect(withMove(d, 0, 5)).toBe(d);
  });

  it("removes one copy of a duplicated track and updates the totals", () => {
    const d = withRemoved(detail(["a", "b", "a"]), [2]);
    expect(ids(d)).toEqual(["a", "b"]);
    expect(d.durationMs).toBe(2000);
    expect(d.playlist.trackCount).toBe(2);
  });
});

describe("serial", () => {
  it("runs a playlist's edits in order and reports when none are pending", async () => {
    const order: string[] = [];
    // tsconfig's lib predates Promise.withResolvers.
    let open = () => {};
    const gate = new Promise<void>((resolve) => (open = resolve));
    const first = serial("p.s", () => gate.then(() => void order.push("first")));
    const second = serial("p.s", async () => void order.push("second"));
    expect(editing("p.s")).toBe(true);
    await Promise.resolve();
    expect(order).toEqual([]);
    open();
    await first;
    expect(editing("p.s")).toBe(true); // the second is still queued
    await second;
    expect(order).toEqual(["first", "second"]);
    expect(editing("p.s")).toBe(false);
  });

  it("keeps going after a failed edit", async () => {
    await expect(serial("p.f", () => Promise.reject(new Error("boom")))).rejects.toThrow("boom");
    await expect(serial("p.f", async () => "ok")).resolves.toBe("ok");
    expect(editing("p.f")).toBe(false);
  });
});
