import type { Item, Station } from "@bindings/services";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { KeyboardEventHandler, ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";
import { registerItemPlayer } from "@/shared/items/play";

import { ItemRows } from "./ItemRows";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    tabIndex,
    onKeyDown,
    "data-opener": opener,
  }: {
    children: ReactNode;
    tabIndex?: number;
    onKeyDown?: KeyboardEventHandler;
    "data-opener"?: boolean;
  }) => (
    <a href="/" tabIndex={tabIndex} onKeyDown={onKeyDown} data-opener={opener}>
      {children}
    </a>
  ),
}));
vi.mock("@/shared/actions/ActionMenu", () => ({ ItemActionMenu: () => null }));

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});
afterEach(cleanup);

function station(id: string, name: string): Item {
  return {
    kind: "station",
    id,
    name,
    subtitle: "",
    caption: "",
    art: { small: "", medium: "", large: "" },
    wide: { small: "", medium: "", large: "" },
    color: "",
    explicit: false,
    url: "",
    station: { id, name, kind: "tracks", playable: true } as unknown as Station,
  } as Item;
}

const items = [station("ra.1", "Energy"), station("ra.2", "Focus"), station("ra.3", "Relax")];
const opener = (name: string) => screen.getByText(name).closest("a")!;

describe("ItemRows", () => {
  it("is one Tab stop, and the arrow keys, Home and End move between rows", () => {
    // jsdom lays nothing out: one column, so Down is the next row.
    render(<ItemRows items={items} label="Find Your Mood" rows={3} />, { wrapper: I18nRoot });
    expect(items.map((i) => opener(i.name).tabIndex)).toEqual([0, -1, -1]);

    opener("Energy").focus();
    fireEvent.keyDown(opener("Energy"), { key: "ArrowDown" });
    expect(document.activeElement).toBe(opener("Focus"));
    // The row focused last is the list's Tab stop.
    expect(items.map((i) => opener(i.name).tabIndex)).toEqual([-1, 0, -1]);

    fireEvent.keyDown(opener("Focus"), { key: "End" });
    expect(document.activeElement).toBe(opener("Relax"));
    fireEvent.keyDown(opener("Relax"), { key: "ArrowDown" });
    expect(document.activeElement).toBe(opener("Relax"));
    fireEvent.keyDown(opener("Relax"), { key: "Home" });
    expect(document.activeElement).toBe(opener("Energy"));
  });

  it("plays the focused row with Space", () => {
    const play = vi.fn();
    registerItemPlayer("station", play);
    render(<ItemRows items={items} label="Find Your Mood" rows={3} />, { wrapper: I18nRoot });
    fireEvent.keyDown(opener("Focus"), { key: " " });
    expect(play).toHaveBeenCalledWith(items[1]);
  });

  it("shows `rows` rows until opened", () => {
    const { rerender } = render(<ItemRows items={items} label="Find Your Mood" rows={2} />, {
      wrapper: I18nRoot,
    });
    expect(screen.queryByText("Relax")).toBeNull();
    rerender(<ItemRows items={items} label="Find Your Mood" rows={2} open />);
    expect(screen.getByText("Relax")).toBeTruthy();
  });
});
