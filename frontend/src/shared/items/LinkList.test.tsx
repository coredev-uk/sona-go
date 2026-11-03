import type { Item } from "@bindings/services";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { openWithoutPage } from "./cover";
import { LinkList } from "./LinkList";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    search,
    children,
    className,
  }: {
    to: string;
    params?: { id: string };
    search?: { genre?: string };
    children: ReactNode;
    className?: string;
  }) => (
    <a
      className={className}
      href={`${to.replace("$id", params?.id ?? "")}${search?.genre ? `?genre=${search.genre}` : ""}`}
    >
      {children}
    </a>
  ),
}));
vi.mock("./cover", () => ({
  openWithoutPage: vi.fn(),
  playsWhenOpened: (i: Item) => i.kind === "station",
}));

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});
afterEach(cleanup);

const link = (kind: string, id: string, name: string, url = ""): Item =>
  ({ kind, id, name, url }) as unknown as Item;

const items = [
  link("link", "", "Concerts", "https://music.apple.com/concerts"),
  link("room", "6456176470", "Browse by Genre"),
  link("charts", "", "Charts"),
  link("grouping", "10023", "Music Videos"),
];

function renderList(title: string) {
  return render(<LinkList title={title} items={items} />, { wrapper: I18nRoot });
}

describe("LinkList", () => {
  it("lists every destination in Apple's order, pages in Sona as links", () => {
    renderList("More to Explore");
    const list = screen.getByRole("list");
    const tiles = within(list).getAllByRole("listitem");
    expect(tiles.map((t) => t.textContent)).toEqual([
      "Concerts",
      "Browse by Genre",
      "Charts",
      "Music Videos",
    ]);
    expect(screen.getAllByRole("link").map((a) => [a.textContent, a.getAttribute("href")])).toEqual(
      [
        ["Browse by Genre", "/room/6456176470"],
        ["Charts", "/charts"],
        ["Music Videos", "/grouping/10023"],
      ],
    );
  });

  it("opens a web-only destination in the browser and says so in its name", () => {
    renderList("More to Explore");
    const concerts = screen.getByRole("button", { name: /^Concerts/ });
    expect(concerts.querySelector("[aria-label]")?.getAttribute("aria-label")).toBe(
      "Opens in your browser",
    );
    fireEvent.click(concerts);
    expect(openWithoutPage).toHaveBeenCalledWith(items[0]);
  });

  it("plays a station with no page: a button named by its text with Play after it, no arrow", () => {
    const station = link("station", "ra.1", "Local FM");
    render(<LinkList title="Stations" items={[station]} />, { wrapper: I18nRoot });
    const tile = screen.getByRole("button", { name: "Local FM Play" });
    expect(tile.querySelector("[aria-label]")).toBeNull();
    fireEvent.click(tile);
    expect(openWithoutPage).toHaveBeenCalledWith(station);
  });

  it("names the region by its heading, and a continuation without one 'More links'", () => {
    const { unmount } = renderList("More to Explore");
    expect(screen.getByRole("region", { name: "More to Explore" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "More to Explore" })).toBeTruthy();
    unmount();
    renderList("");
    expect(screen.getByRole("region", { name: "More links" })).toBeTruthy();
    expect(screen.queryByRole("heading")).toBeNull();
  });
});
