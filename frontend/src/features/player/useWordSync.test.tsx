import type { LyricLine } from "@bindings/services";
import { act, cleanup, render } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useLyricsOffset } from "./lyrics";
import { emptyState, usePlayback } from "./playback";
import { useWordSync } from "./useWordSync";

const motion = vi.hoisted(() => ({ reduced: false }));
vi.mock("./useLyricsView", () => ({ useReducedMotion: () => motion.reduced }));

const w = (text: string, beginMs: number, endMs: number, background = false) => ({
  text,
  beginMs,
  endMs,
  background,
});

function line(beginMs: number, endMs: number, words: LyricLine["words"]): LyricLine {
  return {
    beginMs,
    endMs,
    text: words.map((x) => x.text).join(""),
    backgroundText: "",
    part: "",
    agent: "",
    agentType: "",
    words,
    translation: null,
    transliteration: null,
  };
}

// Held notes (600 ms or more) fill; shorter words light whole. Line 0's
// background vocal (echo) runs on under line 1; line 1 is the last.
const LINES = [
  line(1_000, 2_000, [
    w("alpha ", 1_000, 1_800),
    w("bravo", 1_800, 2_000),
    w("echo", 1_800, 3_000, true),
  ]),
  line(2_500, 3_500, [w("charlie ", 2_500, 3_100), w("delta", 3_100, 3_500)]),
];

function View() {
  const ref = useRef<HTMLDivElement>(null);
  useWordSync(ref, "song", LINES, true, null);
  return (
    <div ref={ref}>
      {LINES.map((l, i) => (
        <p key={i} data-line={i}>
          {l.words.map((x, j) => (
            <span key={j} data-wb={x.beginMs} data-we={x.endMs}>
              {x.text}
            </span>
          ))}
        </p>
      ))}
    </div>
  );
}

const T0 = 1_700_000_000_000;

function play(positionMs: number, state = "playing") {
  act(() =>
    usePlayback.setState({ ...emptyState, state, positionMs, at: Date.now(), durationMs: 60_000 }),
  );
}

function advance(ms: number) {
  act(() => vi.advanceTimersByTime(ms));
}

const word = (text: string) => {
  const el = [...document.querySelectorAll<HTMLElement>("[data-wb]")].find(
    (e) => e.textContent === text,
  )!;
  return { sung: el.dataset.sung ?? "", p: el.style.getPropertyValue("--p") };
};

beforeEach(() => {
  vi.useFakeTimers({
    now: T0,
    toFake: ["setTimeout", "clearTimeout", "requestAnimationFrame", "cancelAnimationFrame", "Date"],
  });
  motion.reduced = false;
  useLyricsOffset.setState({ offsets: {} });
  usePlayback.setState({ ...emptyState });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useWordSync", () => {
  it("fills a held note in steps and lights short words whole", () => {
    play(1_200);
    render(<View />);
    expect(word("alpha ")).toEqual({ sung: "part", p: "0.25" });
    expect(word("bravo")).toEqual({ sung: "", p: "" });

    advance(40); // between steps: nothing is redrawn
    expect(word("alpha ").p).toBe("0.25");
    advance(41); // the next step, FILL_STEP_MS after the first
    expect(Number(word("alpha ").p)).toBeCloseTo(0.351, 2);

    advance(620); // 1.9 s
    expect(word("alpha ").sung).toBe("done");
    expect(word("bravo")).toEqual({ sung: "done", p: "" });
    expect(word("echo").sung).toBe("part");
  });

  it("keeps background vocals running under the next line", () => {
    play(2_600);
    render(<View />);
    expect(word("bravo").sung).toBe("done");
    expect(word("echo").sung).toBe("part");
    expect(word("charlie ").sung).toBe("part");

    advance(510); // 3.11 s: the echo has ended, and its line is let go
    expect(word("echo").sung).toBe("");
    expect(word("alpha ").sung).toBe("");
    expect(word("charlie ").sung).toBe("done");
    expect(word("delta").sung).toBe("done");
  });

  it("lets the last line go once the voice rests", () => {
    play(3_400);
    render(<View />);
    expect(word("delta").sung).toBe("done");
    advance(700); // 4.1 s: past the line's end and its grace
    expect(document.querySelectorAll("[data-sung]")).toHaveLength(0);
  });

  it("holds still while paused and follows seeks", () => {
    play(1_200, "paused");
    render(<View />);
    expect(word("alpha ")).toEqual({ sung: "part", p: "0.25" });
    advance(2_000);
    expect(word("alpha ")).toEqual({ sung: "part", p: "0.25" });

    play(3_200, "paused"); // seek forward
    expect(word("alpha ").sung).toBe("");
    expect(word("charlie ").sung).toBe("done");
    expect(word("delta").sung).toBe("done");

    play(500, "paused"); // seek back before the first line
    expect(document.querySelectorAll("[data-sung]")).toHaveLength(0);
  });

  it("never draws a frame loop, filling or not", () => {
    const raf = vi.spyOn(window, "requestAnimationFrame");
    play(0);
    render(<View />);
    advance(1_001);
    expect(word("alpha ").sung).toBe("part");
    advance(2_000);
    expect(word("charlie ").sung).toBe("part");
    expect(raf).not.toHaveBeenCalled();
  });

  it("lights whole words under reduced motion", () => {
    motion.reduced = true;
    play(1_200);
    render(<View />);
    expect(word("alpha ")).toEqual({ sung: "done", p: "" });
    expect(word("bravo").sung).toBe("");
    advance(650);
    expect(word("bravo")).toEqual({ sung: "done", p: "" });
    expect(word("echo")).toEqual({ sung: "done", p: "" });
  });

  it("moves with the song's lyrics offset", () => {
    play(1_200, "paused");
    render(<View />);
    act(() => useLyricsOffset.getState().nudge("song", 1)); // lyrics 0.5 s later
    expect(word("alpha ").sung).toBe("");
  });
});
