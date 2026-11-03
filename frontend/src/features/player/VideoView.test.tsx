import type { Capabilities, QueueItem } from "@bindings/services";
import { ErrorCode } from "@bindings/services/models";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { keys } from "@/shared/lib/keys";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";
import { TooltipProvider } from "@/shared/ui/tooltip";

import { emptyState, usePlayback } from "./playback";
import { useUI } from "./ui";
import type * as Frames from "./video-frames";
import { VideoView } from "./VideoView";

const svc = vi.hoisted(() => ({
  Toggle: vi.fn(() => Promise.resolve()),
  Prev: vi.fn(() => Promise.resolve()),
  Next: vi.fn(() => Promise.resolve()),
  Seek: vi.fn(() => Promise.resolve()),
  SetVolume: vi.fn(() => Promise.resolve()),
  OpenInAppleMusic: vi.fn(() => Promise.resolve()),
}));
const win = vi.hoisted(() => ({
  IsFullscreen: vi.fn(() => Promise.resolve(false)),
  Fullscreen: vi.fn(() => Promise.resolve()),
  UnFullscreen: vi.fn(() => Promise.resolve()),
}));
const frameLoop = vi.hoisted(() =>
  vi.fn<(signal: AbortSignal, h: Frames.FrameLoopHandlers) => Promise<void>>(() =>
    Promise.resolve(),
  ),
);
const APPLE_URL = "https://music.apple.com/gb/music-video/sledgehammer/1442907386";

vi.mock("@bindings/services", () => ({
  PlaybackService: {
    Toggle: svc.Toggle,
    Prev: svc.Prev,
    Next: svc.Next,
    Seek: svc.Seek,
    SetVolume: svc.SetVolume,
  },
  CatalogService: {
    MusicVideo: () => Promise.resolve({ video: { url: APPLE_URL } }),
    OpenInAppleMusic: svc.OpenInAppleMusic,
  },
}));
vi.mock("@wailsio/runtime", () => ({ Window: win }));
// jsdom has neither WebGL nor a 2D canvas, nor Go's /video/frame: a renderer
// that draws nothing, and a frame loop each test drives.
vi.mock("./video-render", () => ({
  createRenderer: () => ({ draw: vi.fn(), clear: vi.fn(), dispose: vi.fn() }),
}));
vi.mock("./video-frames", async (actual) => ({
  ...(await actual<typeof Frames>()),
  runFrameLoop: frameLoop,
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, onClick }: { children: ReactNode; onClick?: () => void }) => (
    <a href="#" onClick={onClick}>
      {children}
    </a>
  ),
}));

const video: QueueItem = {
  uid: 7,
  id: "1442907386",
  source: "catalog",
  catalogId: "1442907386",
  libraryId: "",
  name: "Sledgehammer",
  artistName: "Peter Gabriel",
  albumName: "",
  albumId: "",
  artistId: "",
  durationMs: 300_000,
  art: { small: "", medium: "", large: "" },
  backdrop: "",
  auto: false,
  live: false,
  added: false,
  explicit: false,
  video: true,
} as QueueItem;

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

beforeEach(() => {
  vi.clearAllMocks();
  usePlayback.setState({
    ...emptyState,
    current: video,
    state: "paused",
    positionMs: 60_000,
    durationMs: 300_000,
    at: Date.now(),
    volume: 0.8,
    controls: { ...emptyState.controls, next: true, previous: true, seek: true },
  });
  useUI.setState({ overlay: "video" });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function show(caps: Capabilities = linuxCapabilities()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(keys.capabilities, caps);
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <VideoView />
      </TooltipProvider>
    </QueryClientProvider>,
    { wrapper: I18nRoot },
  );
  return screen.getByRole("region", { name: "Music video" });
}

const button = (name: string) => screen.getByRole("button", { name });

describe("VideoView", () => {
  it("names every control and starts with focus on the view", () => {
    const view = show();
    expect(document.activeElement).toBe(view);
    expect(view.getAttribute("aria-describedby")).toBeTruthy();
    for (const name of ["Play", "Previous", "Next", "Full screen", "Close video", "Mute"]) {
      expect(button(name)).toBeTruthy();
    }
    const seek = screen.getByRole("slider", { name: "Seek" });
    expect(seek.getAttribute("aria-valuetext")).toBe("1:00 of 5:00");
    expect(screen.getByRole("slider", { name: "Volume" })).toBeTruthy();
    expect(screen.getByText("SD")).toBeTruthy();
    expect(screen.getByText("4:00 left")).toBeTruthy();
  });

  it("plays and pauses with Space and K, seeks 5 s with the arrows, mutes with M", () => {
    const view = show();
    fireEvent.keyDown(view, { key: " " });
    fireEvent.keyDown(view, { key: "k" });
    expect(svc.Toggle).toHaveBeenCalledTimes(2);

    fireEvent.keyDown(view, { key: "ArrowRight" });
    expect(svc.Seek).toHaveBeenLastCalledWith(65_000);
    fireEvent.keyDown(view, { key: "ArrowLeft" });
    expect(svc.Seek).toHaveBeenLastCalledWith(60_000);
    // Shift+arrows are the player's own 10 s seek, left to the shortcut dispatcher.
    fireEvent.keyDown(view, { key: "ArrowLeft", shiftKey: true });
    expect(svc.Seek).toHaveBeenCalledTimes(2);

    fireEvent.keyDown(view, { key: "m" });
    expect(svc.SetVolume).toHaveBeenCalledWith(0);
  });

  it("leaves Space to a focused button", () => {
    show();
    const next = button("Next");
    next.focus();
    fireEvent.keyDown(next, { key: " " });
    expect(svc.Toggle).not.toHaveBeenCalled();
  });

  it("toggles full screen with F; Escape leaves full screen, then closes the view", async () => {
    const view = show();
    fireEvent.keyDown(view, { key: "f" });
    expect(win.Fullscreen).toHaveBeenCalled();
    expect(button("Exit full screen")).toBeTruthy();

    fireEvent.keyDown(view, { key: "Escape" });
    expect(win.UnFullscreen).toHaveBeenCalled();
    expect(useUI.getState().overlay).toBe("video");

    fireEvent.keyDown(screen.getByRole("region", { name: "Music video" }), { key: "Escape" });
    expect(useUI.getState().overlay).toBe("none");
  });

  it("closes from its button, and when the queue moves on to a song", () => {
    show();
    fireEvent.click(button("Close video"));
    expect(useUI.getState().overlay).toBe("none");

    cleanup();
    useUI.setState({ overlay: "video" });
    show();
    act(() => usePlayback.setState({ current: { ...video, uid: 8, video: false } }));
    expect(useUI.getState().overlay).toBe("none");
  });

  it("says it is loading until the first picture", () => {
    usePlayback.setState({ state: "loading" });
    show();
    expect(within(screen.getByRole("status")).getByText("Loading the video…")).toBeTruthy();
  });

  it("explains an HD-only video, with Apple Music as the way to watch it", async () => {
    usePlayback.setState({ state: "error", error: ErrorCode.CodeVideoHDOnly });
    show();
    const alert = screen.getByRole("alert");
    expect(within(alert).getByText("Couldn't play this video")).toBeTruthy();
    expect(within(alert).getByText(/only in HD/)).toBeTruthy();
    // Playing again can't help: no Try again.
    expect(within(alert).queryByRole("button", { name: "Try again" })).toBeNull();
    fireEvent.click(await within(alert).findByRole("button", { name: "Watch in Apple Music" }));
    expect(svc.OpenInAppleMusic).toHaveBeenCalledWith(APPLE_URL);
  });

  it("explains a refused licence, and offers Try again for other failures", () => {
    usePlayback.setState({ state: "error", error: ErrorCode.CodeVideoLicence });
    show();
    expect(within(screen.getByRole("alert")).getByText(/refused the licence/)).toBeTruthy();
    cleanup();

    usePlayback.setState({ state: "error", error: ErrorCode.CodeNetwork });
    show();
    fireEvent.click(within(screen.getByRole("alert")).getByRole("button", { name: "Try again" }));
    expect(svc.Toggle).toHaveBeenCalled();
  });

  it("says where videos don't play", () => {
    show(bareCapabilities("windows"));
    expect(
      within(screen.getByRole("alert")).getByText("Music videos don't play here"),
    ).toBeTruthy();
    expect(screen.queryByText("SD")).toBeNull();
  });

  it("fades the controls while a picture plays untouched, and brings them back on a key", () => {
    vi.useFakeTimers();
    frameLoop.mockImplementationOnce((_signal, h) => {
      const plane = new Uint8Array(4);
      h.onPicture({ seq: 1, width: 2, height: 2, positionMs: 0, y: plane, u: plane, v: plane });
      return Promise.resolve();
    });
    usePlayback.setState({ state: "playing", at: Date.now() });
    const view = show();
    const header = view.querySelector("header")!;
    expect(header.className).not.toContain("opacity-0");
    act(() => vi.advanceTimersByTime(3_000));
    expect(header.className).toContain("opacity-0");
    fireEvent.keyDown(view, { key: "Shift" });
    expect(header.className).not.toContain("opacity-0");
  });

  it("keeps the controls up while it waits for the first picture", () => {
    vi.useFakeTimers();
    usePlayback.setState({ state: "playing", at: Date.now() });
    const view = show();
    act(() => vi.advanceTimersByTime(3_000));
    expect(view.querySelector("header")!.className).not.toContain("opacity-0");
    expect(within(screen.getByRole("status")).getByText("Loading the video…")).toBeTruthy();
  });
});
