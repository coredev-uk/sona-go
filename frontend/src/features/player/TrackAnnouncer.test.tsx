import type { PlaybackState } from "@bindings/services";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { emptyState, usePlayback } from "./playback";
import { TrackAnnouncer } from "./TrackAnnouncer";

const song = (id: string, name: string, artistName = "Coldplay") =>
  ({ id, name, artistName }) as NonNullable<PlaybackState["current"]>;

function snapshot(patch: Partial<PlaybackState>) {
  act(() => usePlayback.getState().apply({ ...emptyState, ...patch }));
}

const said = () => screen.getByRole("status").textContent;

beforeEach(() => {
  usePlayback.getState().apply(emptyState);
  render(<TrackAnnouncer />, { wrapper: I18nRoot });
});
afterEach(cleanup);

describe("TrackAnnouncer", () => {
  it("says the song and artist when a full song starts", () => {
    snapshot({ state: "playing", current: song("a", "Yellow") });
    expect(said()).toBe("Now playing: Yellow by Coldplay");
  });

  it("says the song plays as a preview", () => {
    snapshot({ state: "playing", current: song("a", "Yellow"), preview: true });
    expect(said()).toBe("Now playing a preview: Yellow by Coldplay");
    snapshot({ state: "playing", current: song("b", "Clocks", ""), preview: true });
    expect(said()).toBe("Now playing a preview: Clocks");
  });

  it("waits for the engine's report: a first song loading before it says preview is announced once, as a preview", () => {
    snapshot({ state: "loading", current: song("a", "Yellow"), preview: false });
    expect(said()).toBe("");
    snapshot({ state: "playing", current: song("a", "Yellow"), preview: true });
    expect(said()).toBe("Now playing a preview: Yellow by Coldplay");
    expect(screen.getByRole("status").children).toHaveLength(1);
  });
});
