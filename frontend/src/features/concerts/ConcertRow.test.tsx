import type { Concert } from "@bindings/services";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { ConcertRow } from "./ConcertRow";

const open = vi.hoisted(() => vi.fn());
vi.mock("@wailsio/runtime", () => ({ Browser: { OpenURL: open } }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    params,
  }: {
    children: ReactNode;
    to: string;
    params?: { id: string };
  }) => <a href={to.replace("$id", params?.id ?? "")}>{children}</a>,
}));

beforeAll(() => {
  // Radix's popper measures with ResizeObserver, which jsdom lacks.
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

afterEach(() => {
  cleanup();
  open.mockClear();
});

const concert = (over: Partial<Concert> = {}): Concert =>
  ({
    id: "ce.1",
    name: "Example Band with Opener Duo @ Example Hall",
    lineup: "Example Band with Opener Duo",
    artists: [{ id: "900001", name: "Example Band", art: { small: "", medium: "", large: "" } }],
    startsAt: Date.parse("2027-04-14T17:00:00Z"),
    endsAt: 0,
    timezone: "Europe/London",
    venue: { name: "Example Hall", city: "London", region: "", address: "", countryCode: "GB" },
    tickets: [{ vendor: "Example Tickets", url: "https://tickets.example.com/e/1" }],
    distanceKm: 0,
    yourArtist: false,
    ...over,
  }) as Concert;

const row = (c: Concert, props: { date?: boolean; artist?: boolean } = {}) =>
  render(
    <ul>
      <ConcertRow concert={c} {...props} />
    </ul>,
    { wrapper: I18nRoot },
  );

describe("ConcertRow", () => {
  it("names the line-up as a link to the headliner, with the venue and city", () => {
    row(concert());
    const link = screen.getByRole("link", { name: "Example Band with Opener Duo" });
    expect(link.getAttribute("href")).toBe("/artist/900001");
    expect(screen.getByText("Example Hall · London")).toBeTruthy();
  });

  it("leads with the venue on the artist's own page, naming the line-up only when others play", () => {
    row(concert(), { artist: true });
    expect(screen.getByText("Example Hall")).toBeTruthy();
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText("Example Band with Opener Duo")).toBeTruthy();
    cleanup();
    row(concert({ lineup: "Example Band" }), { artist: true });
    expect(screen.queryByText("Example Band")).toBeNull();
  });

  it("opens the only ticket seller in the browser, naming it for screen readers", () => {
    row(concert());
    const button = screen.getByRole("button", {
      name: /Get tickets for Example Band with Opener Duo from Example Tickets/,
    });
    fireEvent.click(button);
    expect(open).toHaveBeenCalledExactlyOnceWith("https://tickets.example.com/e/1");
  });

  it("offers several sellers as a menu", () => {
    row(
      concert({
        tickets: [
          { vendor: "First Seller", url: "https://first.example.com/1" },
          { vendor: "Second Seller", url: "https://second.example.com/2" },
        ],
      }),
    );
    const trigger = screen.getByRole("button", {
      name: "Get tickets for Example Band with Opener Duo",
    });
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "Enter" });
    const item = screen.getByRole("menuitem", { name: /Second Seller/ });
    fireEvent.click(item);
    expect(open).toHaveBeenCalledExactlyOnceWith("https://second.example.com/2");
  });

  it("shows no ticket control when Apple lists no seller", () => {
    row(concert({ tickets: [] }));
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("marks a show by an artist the user follows", () => {
    row(concert({ yourArtist: true }));
    expect(screen.getByText("Your artist")).toBeTruthy();
  });

  it("puts the date first where there is no day heading", () => {
    row(concert(), { date: true });
    expect(screen.getByText("14")).toBeTruthy();
    expect(screen.getByText("Apr")).toBeTruthy();
  });
});
