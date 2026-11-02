import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { Equaliser } from "@/shared/tracks/Equaliser";

import { emptyState, usePlayback } from "./playback";
import { NowPlayingProvider } from "./slots";
import { useUI } from "./ui";

afterEach(() => {
  cleanup();
  useUI.setState({ overlay: "none" });
  usePlayback.setState({ ...emptyState });
});

describe("NowPlayingProvider", () => {
  it("holds the equaliser still while the overlay covers the page", () => {
    usePlayback.setState({ ...emptyState, state: "playing" });
    render(
      <NowPlayingProvider>
        <Equaliser playing />
      </NowPlayingProvider>,
      { wrapper: I18nRoot },
    );
    const eq = screen.getByRole("img", { name: "Now playing" });
    expect(eq.classList.contains("eq-live")).toBe(true);

    act(() => useUI.setState({ overlay: "now-playing" }));
    expect(eq.classList.contains("eq-live")).toBe(false);
    // Still named for what it marks: only the movement stops.
    expect(eq.getAttribute("aria-label")).toBe("Now playing");

    act(() => useUI.setState({ overlay: "room" }));
    expect(eq.classList.contains("eq-live")).toBe(false);

    act(() => useUI.setState({ overlay: "none" }));
    expect(eq.classList.contains("eq-live")).toBe(true);
  });
});
