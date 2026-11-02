import type { MusicVideo, Song } from "@bindings/services";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { playVideos, queueItems } from "./commands";
import { useUI } from "./ui";

const svc = vi.hoisted(() => ({
  PlayItems: vi.fn(() => Promise.resolve()),
  PlayNext: vi.fn(() => Promise.resolve()),
  Append: vi.fn(() => Promise.resolve()),
  Insert: vi.fn(() => Promise.resolve()),
}));
const playItems = svc.PlayItems;

vi.mock("@bindings/services", () => ({ PlaybackService: svc }));
vi.mock("sonner", () => ({ toast: Object.assign(vi.fn(), { error: vi.fn() }) }));

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
