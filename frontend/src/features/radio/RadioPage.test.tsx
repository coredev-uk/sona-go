import type { EditorialPage, Item, Section, Station } from "@bindings/services";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { RadioPage } from "./RadioPage";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="/" className={className}>
      {children}
    </a>
  ),
}));
vi.mock("./commands", () => ({ playStation: vi.fn() }));
vi.mock("@/shared/items/EditorialSections", () => ({
  EditorialSections: ({ sections }: { sections: Section[] }) =>
    sections.map((s) => (
      <section key={s.id} aria-label={s.title}>
        <h2>{s.title}</h2>
      </section>
    )),
}));
vi.mock("@/shared/components/CoverShelf", () => ({
  CoverShelf: ({ title }: { title: string }) => <h2>{title}</h2>,
}));
vi.mock("./OnAir", () => ({
  OnAirBoard: ({ stations }: { stations: Station[] }) => (
    <ul aria-label="board">
      {stations.map((s) => (
        <li key={s.id}>{s.name}</li>
      ))}
    </ul>
  ),
  OnAirSkeleton: () => null,
}));

type Q<T> = { data?: T; isPending: boolean; isError: boolean; error?: unknown };
const ok = <T,>(data: T): Q<T> => ({ data, isPending: false, isError: false });
const failed = <T,>(): Q<T> => ({ isPending: false, isError: true, error: new Error("offline") });

const h = vi.hoisted(() => ({
  auth: "signed_out",
  live: undefined as unknown as Q<Station[]>,
  recent: undefined as unknown as Q<Station[]>,
  grouping: undefined as unknown as Q<EditorialPage>,
}));
vi.mock("@/features/auth", () => ({
  useAuth: () => ({ data: { state: h.auth } }),
  hasAccount: (s: string) => s === "signed_in",
  isSignedOut: (s: string) => s === "signed_out",
}));
vi.mock("./queries", () => ({
  useLiveStations: () => ({ ...h.live, refetch: vi.fn() }),
  useRecentStations: () => ({ ...h.recent, refetch: vi.fn() }),
  useRadioGrouping: () => ({ ...h.grouping, refetch: vi.fn() }),
}));

const station = (id: string, kind: string): Station =>
  ({ id, name: id, kind, art: { small: "", medium: "", large: "" } }) as Station;
const item = (s: Station): Item => ({ kind: "station", id: s.id, station: s }) as Item;
const section = (id: string, items: Item[], kind = "shelf"): Section =>
  ({ id, kind, title: id, items, text: "" }) as Section;
const live = [station("Apple Music 1", "live"), station("Hits", "live")];

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});
beforeEach(() => {
  h.auth = "signed_out";
  h.live = ok(live);
  h.recent = ok([]);
  h.grouping = ok({
    sections: [
      section("", live.map(item), "hero"),
      section("On Air Now", live.map(item), "live"),
      section("Latest Episodes", [item(station("ep", "episode"))]),
      section("Local Broadcasters", [item(station("KISS", "broadcast"))], "stations"),
      section("Stations by Genre", [item(station("Pop", "tracks"))]),
    ],
  } as unknown as EditorialPage);
});
afterEach(cleanup);

const headings = () => screen.getAllByRole("heading").map((e) => e.textContent);

describe("RadioPage", () => {
  it("leads with the live board and the local broadcasters, then the bands", () => {
    render(<RadioPage />, { wrapper: I18nRoot });
    expect(headings()).toEqual([
      "Radio",
      "Live now",
      "Local Broadcasters",
      "Stations",
      "Your stations",
      "Stations by Genre",
      "Shows and episodes",
      "Latest Episodes",
    ]);
    // Apple's own rows of the live stations are left out: the board lists them.
    expect(screen.queryByRole("region", { name: "On Air Now" })).toBeNull();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeTruthy();
  });

  it("puts the user's stations first in the Stations band when signed in", () => {
    h.auth = "signed_in";
    h.recent = ok([station("My Station", "tracks")]);
    render(<RadioPage />, { wrapper: I18nRoot });
    expect(headings().slice(3, 6)).toEqual(["Stations", "Your stations", "Stations by Genre"]);
    expect(screen.queryByRole("button", { name: "Sign in" })).toBeNull();
  });

  it("keeps Apple's live shelves when Sona couldn't list the live stations", () => {
    h.live = failed();
    render(<RadioPage />, { wrapper: I18nRoot });
    expect(screen.getByText("Couldn't load the live stations")).toBeTruthy();
    expect(screen.getByRole("region", { name: "On Air Now" })).toBeTruthy();
  });

  it("says once that nothing loaded", () => {
    h.live = failed();
    h.grouping = failed();
    render(<RadioPage />, { wrapper: I18nRoot });
    expect(screen.getByText("Couldn't load Apple Music's radio")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /Retry/ })).toHaveLength(1);
    expect(screen.queryByText("Couldn't load the live stations")).toBeNull();
  });
});
