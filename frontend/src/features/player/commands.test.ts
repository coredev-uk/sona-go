import type { MusicVideo } from "@bindings/services";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { playVideos } from "./commands";
import { useUI } from "./ui";

const playItems = vi.hoisted(() => vi.fn(() => Promise.resolve()));

vi.mock("@bindings/services", () => ({ PlaybackService: { PlayItems: playItems } }));

const video = (id: string, source: string) => ({ id, source }) as unknown as MusicVideo;

beforeEach(() => {
  playItems.mockClear();
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
