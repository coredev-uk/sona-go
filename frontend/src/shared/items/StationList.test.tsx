import type { OnAir, QueueItem, Station } from "@bindings/services";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { NowPlayingContext } from "@/shared/lib/now-playing";

import { StationList } from "./StationList";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="/" className={className}>
      {children}
    </a>
  ),
}));

afterEach(cleanup);

function station(id: string, name: string, provider = ""): Station {
  return {
    id,
    name,
    provider,
    tagline: "",
    kind: "broadcast",
    hasPage: false,
    playable: true,
    unavailable: "",
    art: { small: "", medium: "", large: "" },
  } as unknown as Station;
}

vi.mock("@/shared/actions/ActionMenu", () => ({
  ItemActionMenu: ({ name }: { name: string }) => <button type="button">More for {name}</button>,
}));
const toggle = vi.hoisted(() => vi.fn());
vi.mock("@bindings/services", async (orig) => ({
  ...(await orig<typeof import("@bindings/services")>()),
  PlaybackService: { Toggle: toggle },
}));

const capital = station("ra.1345165422", "Capital");
const heart = station("ra.1", "Heart UK", "TuneIn");

function renderList(
  current: string | null,
  onPlay = vi.fn(),
  stations: Station[] = [capital, heart],
  onAir: OnAir | null = null,
) {
  render(
    <NowPlayingContext.Provider
      value={{
        current: current ? ({ id: current } as QueueItem) : null,
        playing: true,
        onAir,
        covered: false,
      }}
    >
      <StationList label="Local Broadcasters" stations={stations} onPlay={onPlay} />
    </NowPlayingContext.Provider>,
    { wrapper: I18nRoot },
  );
  return onPlay;
}

describe("StationList", () => {
  beforeEach(() => toggle.mockClear());

  it("lists each station with its provider and a play control", () => {
    renderList(null);
    expect(screen.getByRole("list", { name: "Local Broadcasters" })).toBeTruthy();
    expect(screen.getByText("TuneIn")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Capital Play" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Heart UK Play" })).toBeTruthy();
  });

  it("plays a station with no page when it is opened, and offers its menu", () => {
    const onPlay = renderList(null);
    // No link to a page, and the name is itself the play button.
    expect(screen.queryByRole("link")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Capital Play" }));
    expect(onPlay).toHaveBeenCalledWith(capital);
    expect(screen.getByRole("button", { name: "More for Capital" })).toBeTruthy();
  });

  it("links a station that has a page and plays it from its own button", () => {
    const apple = { ...capital, id: "ra.9", name: "Apple FM", hasPage: true } as Station;
    const onPlay = renderList(null, vi.fn(), [apple]);
    expect(screen.getByRole("link", { name: "Apple FM" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Play Apple FM" }));
    expect(onPlay).toHaveBeenCalledWith(apple);
    expect(document.activeElement?.textContent).toBe("Apple FM");
  });

  it("marks the station playing, pauses it from its name and shows what is on air", () => {
    renderList(capital.id, vi.fn(), [capital, heart], {
      title: "Sample Song",
      artist: "Sample Artist",
      art: { small: "", medium: "", large: "" },
    });
    expect(screen.queryByRole("button", { name: "Capital Play" })).toBeNull();
    expect(screen.getByText("Sample Artist – Sample Song")).toBeTruthy();
    // The station that is not playing keeps its tagline line.
    expect(screen.getByText("TuneIn")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Capital Pause" }));
    expect(toggle).toHaveBeenCalledTimes(1);
  });
});
