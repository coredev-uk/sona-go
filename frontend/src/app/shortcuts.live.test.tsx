import type { QueueItem } from "@bindings/services";
import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { playerModule, usePlayback } from "@/features/player";

import { useShortcuts } from "./shortcuts";

const playerShortcuts = playerModule.shortcuts ?? [];
const empty = usePlayback.getState();

const svc = vi.hoisted(() => ({
  Toggle: vi.fn(() => Promise.resolve()),
  Next: vi.fn(() => Promise.resolve()),
  Prev: vi.fn(() => Promise.resolve()),
  Seek: vi.fn(() => Promise.resolve()),
  SetShuffle: vi.fn(() => Promise.resolve()),
  SetRepeat: vi.fn(() => Promise.resolve()),
  SetVolume: vi.fn(() => Promise.resolve()),
}));
const toast = vi.hoisted(() => Object.assign(vi.fn(), { error: vi.fn() }));

vi.mock("@bindings/services", () => ({ PlaybackService: svc }));
vi.mock("sonner", () => ({ toast }));
vi.mock("@tanstack/react-router", async (actual) => ({
  ...(await actual<typeof import("@tanstack/react-router")>()),
  useNavigate: () => vi.fn(),
  useRouter: () => ({ history: {} }),
}));
vi.mock("@/shared/shortcuts/registry", async (actual) => ({
  ...(await actual<typeof import("@/shared/shortcuts/registry")>()),
  registeredShortcuts: () => playerShortcuts,
}));
vi.mock("@/shared/shortcuts/bindings", async () => {
  const { resolveBindings: resolve } = await import("@/shared/shortcuts/registry");
  return { useShortcutBindings: () => resolve(playerShortcuts, {}) };
});
vi.mock("@/shared/lib/capabilities", () => ({
  currentCapabilities: () => ({ playback: { volume: true } }),
}));

const REASON = "Not available while listening to a live station";

const item = (live: boolean) =>
  ({
    uid: 1,
    id: live ? "ra.1445918457" : "1440857781",
    source: "catalog",
    catalogId: live ? "" : "1440857781",
    libraryId: "",
    name: live ? "KISS" : "Song",
    live,
    durationMs: live ? 0 : 180_000,
  }) as unknown as QueueItem;

/** What Go's snapshot says of the controls while `current` plays. */
function playing(current: QueueItem) {
  usePlayback.setState({
    ...empty,
    state: "playing",
    current,
    durationMs: current.durationMs,
    at: Date.now(),
    controls: current.live
      ? {
          previous: false,
          next: false,
          seek: false,
          shuffle: false,
          repeat: false,
          autoPlay: false,
          queue: false,
        }
      : {
          previous: true,
          next: true,
          seek: true,
          shuffle: true,
          repeat: true,
          autoPlay: true,
          queue: true,
        },
  });
}

function press(key: string, init: KeyboardEventInit = {}) {
  const e = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
  window.dispatchEvent(e);
  return e;
}

beforeEach(() => {
  vi.clearAllMocks();
  renderHook(() => useShortcuts());
});

afterEach(() => usePlayback.setState(empty, true));

describe("player keys on a live station", () => {
  const keysThatDontApply: [string, KeyboardEventInit][] = [
    ["next", { key: "ArrowRight", ctrlKey: true }],
    ["previous", { key: "ArrowLeft", ctrlKey: true }],
    ["seek ahead", { key: "ArrowRight", shiftKey: true }],
    ["seek back", { key: "ArrowLeft", shiftKey: true }],
    ["shuffle", { key: "s" }],
    ["repeat", { key: "r" }],
  ];

  it.each(keysThatDontApply)("%s takes the key, does nothing and says why", (_, init) => {
    playing(item(true));
    const e = press(init.key ?? "", init);
    expect(e.defaultPrevented).toBe(true);
    for (const call of Object.values(svc)) expect(call).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast).toHaveBeenCalledWith(REASON, { id: "command-unavailable" });
  });

  it("says it once while the key is held", () => {
    playing(item(true));
    press("ArrowRight", { shiftKey: true });
    press("ArrowRight", { shiftKey: true, repeat: true });
    press("ArrowRight", { shiftKey: true, repeat: true });
    expect(toast).toHaveBeenCalledTimes(1);
  });

  it("leaves play and pause and the volume to work", () => {
    playing(item(true));
    press(" ", { code: "Space" });
    press("ArrowUp", { ctrlKey: true });
    expect(svc.Toggle).toHaveBeenCalledTimes(1);
    expect(svc.SetVolume).toHaveBeenCalledTimes(1);
    expect(toast).not.toHaveBeenCalled();
  });
});

describe("player keys on songs", () => {
  it("run as ever, without a notification", () => {
    playing(item(false));
    press("ArrowRight", { ctrlKey: true });
    press("ArrowLeft", { ctrlKey: true });
    press("s");
    press("r");
    press("ArrowRight", { shiftKey: true });
    expect(svc.Next).toHaveBeenCalledTimes(1);
    expect(svc.Prev).toHaveBeenCalledTimes(1);
    expect(svc.SetShuffle).toHaveBeenCalledTimes(1);
    expect(svc.SetRepeat).toHaveBeenCalledTimes(1);
    expect(svc.Seek).toHaveBeenCalledTimes(1);
    expect(toast).not.toHaveBeenCalled();
  });
});

describe("the command palette's view of the commands", () => {
  it("lists what applies: a live station leaves out skipping, seeking, shuffle and repeat", () => {
    const ids = (live: boolean) => {
      playing(item(live));
      return playerShortcuts.filter((s) => !s.unavailable?.()).map((s) => s.id);
    };
    const gone = [
      "player.next",
      "player.previous",
      "player.seek-forward",
      "player.seek-back",
      "player.shuffle",
      "player.repeat",
    ];
    expect(ids(false)).toEqual(expect.arrayContaining(gone));
    const live = ids(true);
    for (const id of gone) expect(live).not.toContain(id);
    expect(live).toEqual(
      expect.arrayContaining(["player.toggle", "player.volume-up", "player.mute"]),
    );
  });
});
