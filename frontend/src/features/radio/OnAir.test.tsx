import type { Station, StationDetail, StationEvent } from "@bindings/services";
import { StationUnavailable } from "@bindings/services/models";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { clockTime } from "@/shared/lib/format";

import { playStation } from "./commands";
import { OnAirBoard } from "./OnAir";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="/" className={className}>
      {children}
    </a>
  ),
}));

const h = vi.hoisted(() => ({
  details: new Map<string, StationDetail>(),
  current: null as { id: string } | null,
  playing: false,
}));
vi.mock("./queries", () => ({
  useStations: (ids: string[]) => ids.map((id) => ({ data: h.details.get(id) })),
}));
vi.mock("./commands", () => ({ playStation: vi.fn() }));
vi.mock("@/shared/actions/ActionMenu", () => ({
  ItemActionMenu: ({ name }: { name: string }) => <button type="button">More for {name}</button>,
}));
vi.mock("@bindings/services", async (orig) => ({
  ...(await orig<typeof import("@bindings/services")>()),
  PlaybackService: { Toggle: vi.fn() },
}));
vi.mock("@/shared/lib/now-playing", () => ({
  useNowPlaying: () => ({ current: h.current, playing: h.playing }),
}));

const hour = 3600_000;
const t0 = new Date(2026, 10, 2, 10, 0).getTime();

function station(id: string, name: string, extra: Partial<Station> = {}): Station {
  return {
    id,
    name,
    kind: "live",
    hasPage: true,
    playable: true,
    unavailable: "",
    tagline: `${name} tagline`,
    art: { small: "", medium: `/art/${id}/station.jpg`, large: "" },
    ...extra,
  } as Station;
}

function ev(title: string, start: number, end: number, description = ""): StationEvent {
  return {
    id: `${title}-${start}`,
    title,
    description,
    startMs: start,
    endMs: end,
    art: { small: "", medium: `/art/${title.length}/show.jpg`, large: "" },
  };
}

function renderBoard(stations: Station[]) {
  return render(<OnAirBoard stations={stations} />, { wrapper: I18nRoot });
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(t0 + 15 * 60_000);
  h.details.clear();
  h.current = null;
  h.playing = false;
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("OnAirBoard", () => {
  it("shows the programme on air, how long is left of it and the next named programme", () => {
    const one = station("ra.1", "Apple Music 1");
    h.details.set("ra.1", {
      station: one,
      schedule: [
        ev("The Zane Lowe Show", t0, t0 + hour),
        ev("Apple Music 1", t0 + hour, t0 + 2 * hour),
        ev("The Matt Wilkinson Show", t0 + 2 * hour, t0 + 3 * hour),
      ],
    } as StationDetail);
    renderBoard([one]);
    const row = screen.getByRole("listitem");
    expect(within(row).getByText("The Zane Lowe Show")).toBeTruthy();
    expect(row.textContent).toContain(`${clockTime(t0)}–${clockTime(t0 + hour)}`);
    const bar = within(row).getByRole("progressbar", { name: "Time through this programme" });
    expect(bar.getAttribute("aria-valuenow")).toBe("25");
    expect(bar.getAttribute("aria-valuetext")).toMatch(/45 min.* left/);
    // The station's own hour comes between, so the show is "later", not "next".
    expect(row.textContent).toContain(
      `Later at ${clockTime(t0 + 2 * hour)}: The Matt Wilkinson Show`,
    );
    // The schedule's own art, not the station's.
    expect(row.querySelector("img")?.getAttribute("src")).toContain("/show.jpg");
  });

  it("counts the station's own hours down to the next named show", () => {
    const one = station("ra.1", "Apple Music 1");
    h.details.set("ra.1", {
      station: one,
      schedule: [
        ev("Apple Music 1", t0, t0 + hour, "New music."),
        ev("Apple Music 1", t0 + hour, t0 + 2 * hour),
        ev("The Ebro Show", t0 + 2 * hour, t0 + 3 * hour),
      ],
    } as StationDetail);
    renderBoard([one]);
    const row = screen.getByRole("listitem");
    expect(within(row).getByText("New music.")).toBeTruthy();
    expect(row.textContent).toContain(`${clockTime(t0)}–${clockTime(t0 + 2 * hour)}`);
    expect(row.textContent).toContain(`Next at ${clockTime(t0 + 2 * hour)}: The Ebro Show`);
  });

  it("falls back to the station's tagline for its own hours and without a schedule", () => {
    const hits = station("ra.2", "Hits");
    const chill = station("ra.3", "Chill");
    h.details.set("ra.2", {
      station: hits,
      schedule: [ev("Hits", t0, t0 + hour, "Hits tagline"), ev("Hits", t0 + hour, t0 + 2 * hour)],
    } as StationDetail);
    renderBoard([hits, chill]);
    const [hitsRow, chillRow] = screen.getAllByRole("listitem");
    expect(within(hitsRow).getByText("Hits tagline")).toBeTruthy();
    expect(hitsRow.textContent).not.toContain("Next at");
    expect(within(chillRow).getByText("Chill tagline")).toBeTruthy();
    expect(within(chillRow).queryByRole("progressbar")).toBeNull();
    expect(chillRow.querySelector("img")?.getAttribute("src")).toContain("/station.jpg");
  });

  it("says why a station can't play instead of offering a play button", () => {
    const off = station("ra.4", "Off", {
      playable: false,
      unavailable: StationUnavailable.StationNoWidevine,
    });
    renderBoard([off]);
    const row = screen.getByRole("listitem");
    expect(within(row).getByText(/isn't offered with Widevine/)).toBeTruthy();
    expect(within(row).queryByRole("button")).toBeNull();
  });

  it("plays a station from its row and marks the one playing", () => {
    const one = station("ra.1", "Apple Music 1");
    const hits = station("ra.2", "Hits");
    h.current = { id: "ra.2" };
    h.playing = true;
    renderBoard([one, hits]);
    fireEvent.click(screen.getByRole("button", { name: "Play Apple Music 1" }));
    expect(playStation).toHaveBeenCalledWith("ra.1");
    const hitsRow = screen.getAllByRole("listitem")[1];
    expect(within(hitsRow).queryByRole("button")).toBeNull();
    expect(within(hitsRow).getByRole("img", { name: "Now playing" })).toBeTruthy();
  });

  it("plays a live station that has no schedule instead of opening a page for it", () => {
    const quiet = station("ra.2", "Quiet FM", { hasPage: false });
    h.details.set("ra.2", { station: quiet, schedule: [] } as unknown as StationDetail);
    renderBoard([quiet]);
    expect(screen.queryByRole("link")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Quiet FM Play" }));
    expect(playStation).toHaveBeenCalledWith("ra.2");
    expect(screen.getByRole("button", { name: "More for Quiet FM" })).toBeTruthy();
  });

  it("keeps a page for a live station once its schedule says it has one", () => {
    // The list says no page; the station's own page (its schedule) wins.
    const listed = station("ra.3", "Show FM", { hasPage: false });
    h.details.set("ra.3", {
      station: { ...listed, hasPage: true },
      schedule: [ev("Morning", t0, t0 + hour)],
    } as unknown as StationDetail);
    renderBoard([listed]);
    expect(screen.getByRole("link", { name: "Show FM" })).toBeTruthy();
  });
});
