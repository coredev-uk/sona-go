import type { QueueItem, Station } from "@bindings/services";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

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
    playable: true,
    unavailable: "",
    art: { small: "", medium: "", large: "" },
  } as unknown as Station;
}

const capital = station("ra.1345165422", "Capital");
const heart = station("ra.1", "Heart UK", "TuneIn");

function renderList(current: string | null, onPlay = vi.fn()) {
  render(
    <NowPlayingContext.Provider
      value={{
        current: current ? ({ id: current } as QueueItem) : null,
        playing: true,
        covered: false,
      }}
    >
      <StationList label="Local Broadcasters" stations={[capital, heart]} onPlay={onPlay} />
    </NowPlayingContext.Provider>,
    { wrapper: I18nRoot },
  );
  return onPlay;
}

describe("StationList", () => {
  it("lists each station with its provider and a play control", () => {
    renderList(null);
    expect(screen.getByRole("list", { name: "Local Broadcasters" })).toBeTruthy();
    expect(screen.getByText("TuneIn")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Play Capital" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Play Heart UK" })).toBeTruthy();
  });

  it("plays a station and keeps focus in its row", () => {
    const onPlay = renderList(null);
    fireEvent.click(screen.getByRole("button", { name: "Play Capital" }));
    expect(onPlay).toHaveBeenCalledWith(capital);
    expect(document.activeElement?.textContent).toBe("Capital");
  });

  it("marks the station playing instead of offering to play it", () => {
    renderList(capital.id);
    expect(screen.queryByRole("button", { name: "Play Capital" })).toBeNull();
    expect(screen.getByRole("button", { name: "Play Heart UK" })).toBeTruthy();
  });
});
