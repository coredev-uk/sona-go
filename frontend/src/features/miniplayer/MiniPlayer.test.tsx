import type { PlaybackControls, QueueItem } from "@bindings/services";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { usePlayback } from "@/features/player";
import { I18nRoot } from "@/shared/i18n";

import { MiniPlayer } from "./MiniPlayer";

const svc = vi.hoisted(() => ({
  Toggle: vi.fn(() => Promise.resolve()),
  Next: vi.fn(() => Promise.resolve()),
  Prev: vi.fn(() => Promise.resolve()),
  Seek: vi.fn(() => Promise.resolve()),
}));

vi.mock("@bindings/services", () => ({ PlaybackService: svc, DesktopService: {} }));
// The player's public face, without the lyrics and announcer behind it.
vi.mock("@/features/player", async (actual) => ({
  ...(await actual<typeof import("@/features/player")>()),
  SungLine: () => null,
  TrackAnnouncer: () => null,
}));
vi.mock("@/shared/lib/capabilities", () => ({ useCapabilities: () => undefined }));

const empty = usePlayback.getState();
const REASON = "Not available while listening to a live station";

const station = {
  uid: 1,
  id: "ra.1445918457",
  name: "KISS",
  artistName: "",
  albumName: "",
  art: { small: "", medium: "", large: "" },
  backdrop: "",
  live: true,
  durationMs: 0,
} as unknown as QueueItem;
const song = {
  ...station,
  id: "1440",
  name: "Song",
  live: false,
  durationMs: 180_000,
} as QueueItem;

const controls = (c: Partial<PlaybackControls>): PlaybackControls => ({
  previous: false,
  next: false,
  seek: false,
  shuffle: false,
  repeat: false,
  autoPlay: false,
  queue: false,
  ...c,
});

function show(current: QueueItem, c: PlaybackControls) {
  usePlayback.setState({
    ...empty,
    state: "playing",
    current,
    durationMs: current.durationMs,
    controls: c,
  });
  render(<MiniPlayer />, { wrapper: I18nRoot });
}

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});
beforeEach(() => vi.clearAllMocks());
afterEach(() => {
  cleanup();
  usePlayback.setState(empty, true);
});

describe("the mini player", () => {
  it("keeps Previous and Next on a live station off, each saying why, and Pause working", () => {
    show(station, controls({}));
    for (const name of ["Previous", "Next"]) {
      const b = screen.getByRole("button", { name });
      expect(b.getAttribute("aria-disabled"), name).toBe("true");
      expect(b.getAttribute("title")).toBe(`${name}: ${REASON}`);
      expect(document.getElementById(b.getAttribute("aria-describedby") ?? "")?.textContent).toBe(
        REASON,
      );
      fireEvent.click(b);
    }
    expect(svc.Next).not.toHaveBeenCalled();
    expect(svc.Prev).not.toHaveBeenCalled();
    expect(screen.queryByRole("slider", { name: "Seek" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Pause" }));
    expect(svc.Toggle).toHaveBeenCalledTimes(1);
  });

  it("skips and seeks songs", () => {
    show(song, controls({ previous: true, next: true, seek: true }));
    const next = screen.getByRole("button", { name: "Next" });
    expect(next.getAttribute("aria-disabled")).toBeNull();
    fireEvent.click(next);
    expect(svc.Next).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("slider", { name: "Seek" })).toBeTruthy();
  });
});
