import type { MusicVideo, QueueItem, Song } from "@bindings/services";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { cycleRepeat, playVideos, queueItems, seekBy, toggleShuffle } from "./commands";
import { emptyState, usePlayback } from "./playback";
import { useUI } from "./ui";

const svc = vi.hoisted(() => ({
  PlayItems: vi.fn(() => Promise.resolve()),
  PlayNext: vi.fn(() => Promise.resolve()),
  Append: vi.fn(() => Promise.resolve()),
  Insert: vi.fn(() => Promise.resolve()),
  Seek: vi.fn(() => Promise.resolve()),
  SetShuffle: vi.fn(() => Promise.resolve()),
  SetRepeat: vi.fn(() => Promise.resolve()),
}));
const playItems = svc.PlayItems;

vi.mock("@bindings/services", () => ({ PlaybackService: svc }));
const toast = vi.hoisted(() => Object.assign(vi.fn(), { error: vi.fn() }));
vi.mock("sonner", () => ({ toast }));

const video = (id: string, source: string) => ({ id, source, name: id }) as unknown as MusicVideo;
const song = (id: string, playable = true) =>
  ({ id, source: "catalog", name: id, playable }) as unknown as Song;

beforeEach(() => {
  Object.values(svc).forEach((f) => f.mockClear());
  useUI.setState({ overlay: "none" });
});

describe("playVideos", () => {
  it("queues the videos as music-video refs, from start, and opens the video view", async () => {
    await playVideos([video("1442907386", "catalog"), video("l.abc", "library")], 1);
    expect(playItems).toHaveBeenCalledWith(
      [
        { id: "1442907386", source: "catalog", kind: "music-video" },
        { id: "l.abc", source: "library", kind: "music-video" },
      ],
      1,
      { kind: "", id: "" },
    );
    expect(useUI.getState().overlay).toBe("video");
  });

  it("leaves the view closed when playback doesn't start", async () => {
    playItems.mockImplementationOnce(() => Promise.reject(new Error("no")));
    await playVideos([video("1442907386", "catalog")], 0);
    expect(useUI.getState().overlay).toBe("none");
  });
});

describe("queueItems", () => {
  it("queues songs and music videos in order, videos as music-video refs", async () => {
    await queueItems(
      [
        { kind: "song", target: song("1") },
        { kind: "music-video", target: video("2", "catalog") },
        { kind: "song", target: song("3", false) },
      ],
      "next",
    );
    expect(svc.PlayNext).toHaveBeenCalledWith([
      { id: "1", source: "catalog" },
      { id: "2", source: "catalog", kind: "music-video" },
    ]);
  });

  it("appends a video alone, or inserts it at a queue position", async () => {
    await queueItems([{ kind: "music-video", target: video("l.abc", "library") }], "last");
    expect(svc.Append).toHaveBeenCalledWith([
      { id: "l.abc", source: "library", kind: "music-video" },
    ]);
    await queueItems([{ kind: "music-video", target: video("2", "catalog") }], 3);
    expect(svc.Insert).toHaveBeenCalledWith(3, [
      { id: "2", source: "catalog", kind: "music-video" },
    ]);
  });
});

describe("on a live station", () => {
  const live = { id: "ra.1445918457", name: "KISS", live: true, durationMs: 0 } as QueueItem;
  const controls = {
    previous: false,
    next: false,
    seek: false,
    shuffle: false,
    repeat: false,
    autoPlay: false,
    queue: false,
  };
  const reason = "Not available while listening to a live station";

  beforeEach(() => {
    vi.clearAllMocks();
    usePlayback.setState({ ...emptyState, state: "playing", current: live, controls });
  });
  afterEach(() => usePlayback.setState({ ...emptyState }));

  it("queues nothing: Play next, Play last and a drop on the queue all say why", async () => {
    await queueItems([{ kind: "song", target: song("1") }], "next");
    await queueItems([{ kind: "song", target: song("1") }], "last");
    await queueItems([{ kind: "song", target: song("1") }], 2);
    expect(svc.PlayNext).not.toHaveBeenCalled();
    expect(svc.Append).not.toHaveBeenCalled();
    expect(svc.Insert).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith(reason, { id: "command-unavailable" });
  });

  it("changes neither shuffle nor repeat, and has no position to seek from", () => {
    toggleShuffle();
    cycleRepeat();
    seekBy(10_000);
    expect(svc.SetShuffle).not.toHaveBeenCalled();
    expect(svc.SetRepeat).not.toHaveBeenCalled();
    expect(svc.Seek).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledTimes(3);
  });

  it("leaves the same commands to songs", async () => {
    usePlayback.setState({
      current: { ...live, id: "1440", live: false, durationMs: 60_000 },
      durationMs: 60_000,
      controls: { ...controls, seek: true, shuffle: true, repeat: true, queue: true },
    });
    toggleShuffle();
    cycleRepeat();
    seekBy(10_000);
    await queueItems([{ kind: "song", target: song("1") }], "next");
    expect(svc.SetShuffle).toHaveBeenCalledTimes(1);
    expect(svc.SetRepeat).toHaveBeenCalledTimes(1);
    expect(svc.Seek).toHaveBeenCalledTimes(1);
    expect(svc.PlayNext).toHaveBeenCalledTimes(1);
    expect(toast).not.toHaveBeenCalledWith(reason, expect.anything());
  });
});
