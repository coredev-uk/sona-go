import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { Equaliser } from "@/shared/tracks/Equaliser";

import { emptyState, usePlayback } from "./playback";
import { NowPlayingProvider, QueueAside } from "./slots";
import { useUI } from "./ui";

// The queue's own behaviour is not under test here, only whether the aside is drawn.
vi.mock("./QueuePanel", () => ({ QueuePanel: () => <aside aria-label="Queue" /> }));

afterEach(() => {
  cleanup();
  useUI.setState({ overlay: "none", queueOpen: false });
  usePlayback.setState({ ...emptyState });
});

describe("QueueAside", () => {
  it("steps out while the now-playing view (which has Up next) covers the page", () => {
    useUI.setState({ queueOpen: true });
    render(<QueueAside />);
    expect(screen.queryByRole("complementary", { name: "Queue" })).not.toBeNull();

    act(() => useUI.setState({ overlay: "now-playing" }));
    expect(screen.queryByRole("complementary", { name: "Queue" })).toBeNull();
    // Still open: it comes back when the view closes.
    expect(useUI.getState().queueOpen).toBe(true);

    act(() => useUI.setState({ overlay: "none" }));
    expect(screen.queryByRole("complementary", { name: "Queue" })).not.toBeNull();
  });
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
