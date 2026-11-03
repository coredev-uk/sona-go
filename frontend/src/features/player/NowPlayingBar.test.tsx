import type { PlaybackControls, PlaybackState, QueueItem } from "@bindings/services";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { TooltipProvider } from "@/shared/ui/tooltip";

import { NowPlayingBar } from "./NowPlayingBar";
import { emptyState, usePlayback } from "./playback";

const svc = vi.hoisted(() => ({
  Toggle: vi.fn(() => Promise.resolve()),
  Next: vi.fn(() => Promise.resolve()),
  Prev: vi.fn(() => Promise.resolve()),
  Seek: vi.fn(() => Promise.resolve()),
  SetShuffle: vi.fn(() => Promise.resolve()),
  SetRepeat: vi.fn(() => Promise.resolve()),
  SetVolume: vi.fn(() => Promise.resolve()),
}));

vi.mock("@bindings/services", () => ({
  PlaybackService: svc,
  DesktopService: { ToggleMiniPlayer: vi.fn(() => Promise.resolve()) },
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}));
vi.mock("@/features/equalizer", () => ({ EqualizerButton: () => null }));
vi.mock("@/features/private", () => ({ PrivateButton: () => null }));
vi.mock("@/features/radio", () => ({ useOnAirProgramme: () => undefined }));
vi.mock("@/shared/components/RatingButtons", () => ({ RatingButtons: () => null }));
vi.mock("@/shared/lib/capabilities", () => ({
  useCapabilities: () => ({ playback: { volume: true } }),
  currentCapabilities: () => ({ playback: { volume: true } }),
}));
vi.mock("@/shared/shortcuts/bindings", () => ({ useBinding: () => [] }));

const REASON = "Not available while listening to a live station";

const station = {
  uid: 1,
  id: "ra.1445918457",
  source: "catalog",
  catalogId: "",
  libraryId: "",
  name: "KISS",
  artistName: "",
  albumName: "",
  albumId: "",
  artistId: "",
  durationMs: 0,
  art: { small: "", medium: "", large: "" },
  backdrop: "",
  live: true,
  broadcast: true,
} as unknown as QueueItem;
const song = {
  ...station,
  id: "1440857781",
  catalogId: "1440857781",
  name: "Song",
  durationMs: 180_000,
  live: false,
  broadcast: false,
} as QueueItem;

const all: PlaybackControls = {
  previous: true,
  next: true,
  seek: true,
  shuffle: true,
  repeat: true,
  autoPlay: true,
  queue: true,
};
const none: PlaybackControls = {
  previous: false,
  next: false,
  seek: false,
  shuffle: false,
  repeat: false,
  autoPlay: false,
  queue: false,
};

function show(current: QueueItem, controls: PlaybackControls, extra: Partial<PlaybackState> = {}) {
  usePlayback.setState({
    ...emptyState,
    state: "playing",
    current,
    durationMs: current.durationMs,
    shuffle: true,
    repeat: "all",
    controls,
    ...extra,
  });
  render(
    <TooltipProvider>
      <NowPlayingBar />
    </TooltipProvider>,
    { wrapper: I18nRoot },
  );
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
  usePlayback.setState({ ...emptyState });
});

describe("the player bar on a live station", () => {
  const dont = ["Shuffle off", "Previous", "Next", "Repeat off"];

  it("keeps the controls that don't apply in place, off, each saying why", () => {
    show(station, none);
    for (const name of dont) {
      const b = screen.getByRole("button", { name });
      expect(b.getAttribute("aria-disabled"), name).toBe("true");
      expect(b.hasAttribute("disabled"), `${name} stays focusable`).toBe(false);
      expect(b.getAttribute("aria-pressed")).not.toBe("true");
      const why = document.getElementById(b.getAttribute("aria-describedby") ?? "");
      expect(why?.textContent, name).toBe(REASON);
    }
  });

  it("does nothing when they are pressed", () => {
    show(station, none);
    for (const name of dont) fireEvent.click(screen.getByRole("button", { name }));
    expect(svc.Next).not.toHaveBeenCalled();
    expect(svc.Prev).not.toHaveBeenCalled();
    expect(svc.SetShuffle).not.toHaveBeenCalled();
    expect(svc.SetRepeat).not.toHaveBeenCalled();
  });

  it("has no timeline to seek, and still pauses", () => {
    show(station, none);
    expect(screen.queryByRole("slider", { name: "Seek" })).toBeNull();
    expect(screen.getByText("Live")).toBeTruthy();
    const pause = screen.getByRole("button", { name: "Pause" });
    expect(pause.hasAttribute("disabled")).toBe(false);
    fireEvent.click(pause);
    expect(svc.Toggle).toHaveBeenCalledTimes(1);
  });
});

describe("the player bar on a song", () => {
  it("has them all working, with no reason shown", () => {
    show(song, all);
    for (const name of ["Shuffle on", "Previous", "Next", "Repeat all"]) {
      const b = screen.getByRole("button", { name });
      expect(b.getAttribute("aria-disabled"), name).toBeNull();
      expect(b.getAttribute("aria-describedby"), name).toBeNull();
    }
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(svc.Next).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("slider", { name: "Seek" })).toBeTruthy();
  });

  it("greys Next at the end of a queue without blaming a station", () => {
    show(song, { ...all, next: false });
    const next = screen.getByRole("button", { name: "Next" });
    expect(next.hasAttribute("disabled")).toBe(true);
    expect(next.getAttribute("aria-describedby")).toBeNull();
  });
});

describe("the player bar on a station of songs", () => {
  const hits = { id: "ra.hits", name: "Hits Station", autoPlay: false };

  it("keeps Next, Previous and the seek bar, and switches off shuffle and repeat with the station's reason", () => {
    show(
      song,
      { ...all, shuffle: false, repeat: false, autoPlay: false, queue: false },
      { station: hits },
    );
    for (const name of ["Previous", "Next"]) {
      expect(screen.getByRole("button", { name }).getAttribute("aria-disabled"), name).toBeNull();
    }
    expect(screen.getByRole("slider", { name: "Seek" })).toBeTruthy();
    for (const name of ["Shuffle off", "Repeat off"]) {
      const b = screen.getByRole("button", { name });
      expect(b.getAttribute("aria-disabled"), name).toBe("true");
      const why = document.getElementById(b.getAttribute("aria-describedby") ?? "");
      expect(why?.textContent, name).toBe("Not available while listening to a station");
    }
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(svc.Next).toHaveBeenCalledTimes(1);
  });

  it("leaves AutoPlay's own station alone", () => {
    show(song, all, { station: { ...hits, autoPlay: true } });
    expect(
      screen.getByRole("button", { name: "Shuffle on" }).getAttribute("aria-disabled"),
    ).toBeNull();
  });
});
