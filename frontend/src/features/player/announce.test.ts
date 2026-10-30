import type { PlaybackState } from "@bindings/services";
import { describe, expect, it } from "vitest";

import { announcedTrack } from "./announce";

const track = (id: string) =>
  ({ id, name: `Song ${id}`, artistName: "Artist" }) as NonNullable<PlaybackState["current"]>;

describe("announcedTrack", () => {
  it("announces a new track that starts", () => {
    expect(announcedTrack(undefined, { state: "loading", current: track("a") })?.id).toBe("a");
    expect(announcedTrack("a", { state: "playing", current: track("b") })?.id).toBe("b");
  });

  it("stays quiet for the same track: pause, resume, seek", () => {
    expect(announcedTrack("a", { state: "paused", current: track("a") })).toBeNull();
    expect(announcedTrack("a", { state: "playing", current: track("a") })).toBeNull();
  });

  it("stays quiet for a track restored paused at launch, and with nothing playing", () => {
    expect(announcedTrack(undefined, { state: "paused", current: track("a") })).toBeNull();
    expect(announcedTrack(undefined, { state: "idle", current: track("a") })).toBeNull();
    expect(announcedTrack("a", { state: "idle", current: null })).toBeNull();
  });
});
