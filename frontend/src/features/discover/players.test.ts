import type { Capabilities, Item, MusicVideo } from "@bindings/services";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { actionItemOf } from "@/shared/items/links";
import { canPlay, playItem } from "@/shared/items/play";
import { videoTile } from "@/shared/lib/tiles";
import { bareCapabilities, linuxCapabilities } from "@/shared/testing/capabilities";

import { registerPlayers } from "./players";

const player = vi.hoisted(() => ({ playSongs: vi.fn(), playVideos: vi.fn() }));
const caps = vi.hoisted(() => ({ current: undefined as Capabilities | undefined }));

vi.mock("@/features/player", () => player);
vi.mock("@/shared/lib/capabilities", () => ({ currentCapabilities: () => caps.current }));

const video = {
  id: "l.abc",
  source: "library",
  catalogId: "1442907386",
  libraryId: "l.abc",
  name: "Sledgehammer",
  artistName: "Peter Gabriel",
  explicit: false,
  releaseDate: "1986-04-21",
  art: { small: "", medium: "", large: "" },
} as unknown as MusicVideo;

const item = {
  kind: "music-video",
  id: "1442907386",
  name: "Sledgehammer",
  subtitle: "Peter Gabriel",
  caption: "",
  art: { small: "", medium: "", large: "" },
  wide: { small: "", medium: "", large: "" },
  color: "",
  explicit: false,
  url: "",
  musicVideo: video,
} as unknown as Item;

beforeAll(registerPlayers);
beforeEach(() => {
  player.playVideos.mockClear();
  caps.current = linuxCapabilities();
});

describe("the music video player", () => {
  it("plays a music video card alone, as the queue", () => {
    expect(canPlay(item)).toBe(true);
    playItem(item);
    expect(player.playVideos).toHaveBeenCalledWith([video], 0);
  });

  it("gives video tiles a play control that plays the video, and queue actions", () => {
    const tile = videoTile(video);
    expect(tile.play?.label).toBe("Sledgehammer");
    tile.play?.run();
    expect(player.playVideos).toHaveBeenCalledWith([video], 0);
    expect(tile.target).toEqual({ kind: "music-video", target: video });
    expect(actionItemOf(item)).toEqual({ kind: "music-video", target: video });
  });

  it("shows no play control or actions where videos don't play, or before Go has answered", () => {
    for (const c of [bareCapabilities("windows"), undefined]) {
      caps.current = c;
      expect(canPlay(item)).toBe(false);
      expect(videoTile(video).play).toBeUndefined();
      expect(videoTile(video).target).toBeUndefined();
      expect(actionItemOf(item)).toBeUndefined();
    }
  });

  it("has nothing to play on a card without its video", () => {
    expect(canPlay({ ...item, musicVideo: null } as Item)).toBe(false);
  });
});
