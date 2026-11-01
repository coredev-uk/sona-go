import { describe, expect, it } from "vitest";

import { queueLines } from "./queue-lines";

/** Entries from a pattern: "u" a user's song, "a" a station or AutoPlay song. */
const q = (pattern: string) => [...pattern].map((c) => ({ auto: c === "a" }));
const autoPlay = { autoPlay: true };
const radio = { autoPlay: false };

describe("queueLines", () => {
  it("puts AutoPlay under its heading after the user's songs", () => {
    const got = queueLines(q("uuuaa"), 0, autoPlay, false);
    expect(got.lines).toEqual([
      { kind: "row", qi: 0 },
      { kind: "row", qi: 1 },
      { kind: "row", qi: 2 },
      { kind: "head" },
      { kind: "row", qi: 3 },
      { kind: "row", qi: 4 },
    ]);
    expect(got).toMatchObject({ mine: 2, auto: 2, userEnd: 3, head: 3 });
  });

  it("lists from the current song: played songs are left out", () => {
    const got = queueLines(q("uuuaa"), 2, autoPlay, false);
    expect(got.lines[0]).toEqual({ kind: "row", qi: 2 });
    expect(got).toMatchObject({ mine: 0, auto: 2, userEnd: 3, head: 1 });
  });

  it("keeps songs queued while AutoPlay plays before the rest of it", () => {
    // The current song is AutoPlay's; the user queued one after it.
    const got = queueLines(q("uauaa"), 1, autoPlay, false);
    expect(got.lines.map((l) => (l.kind === "row" ? l.qi : l.kind))).toEqual([1, 2, "head", 3, 4]);
    expect(got).toMatchObject({ mine: 1, auto: 2, userEnd: 3 });
  });

  it("heads a station the user started only after songs they queued", () => {
    expect(queueLines(q("aaa"), 0, radio, false).head).toBe(-1);
    expect(queueLines(q("auaa"), 0, radio, false)).toMatchObject({ head: 2, userEnd: 2 });
  });

  it("ends with the note while AutoPlay is on its way", () => {
    const got = queueLines(q("uu"), 0, null, true);
    expect(got.lines.at(-1)).toEqual({ kind: "note" });
    expect(got).toMatchObject({ mine: 1, auto: 0, userEnd: 2, head: -1 });
  });
});
