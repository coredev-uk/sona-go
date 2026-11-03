import type { Item } from "@bindings/services";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { FrontPage } from "./FrontPage";
import { ROTATE_MS } from "./useLeadRotation";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="/" className={className}>
      {children}
    </a>
  ),
}));
vi.mock("@/shared/items/ItemMenu", () => ({ ItemMenu: () => null }));
vi.mock("@/shared/items/play", () => ({ canPlay: () => false, playItem: vi.fn() }));
vi.mock("@/shared/items/cover", () => ({ openWithoutPage: vi.fn() }));

const prefs = vi.hoisted(() => ({ reduceEffects: false, reducedMotion: false }));
vi.mock("@/features/settings", () => ({
  useAppearance: (select: (s: { reduceEffects: boolean }) => boolean) => select(prefs),
}));

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
  // The content column of a 1440px window.
  Object.defineProperty(HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get: () => 1200,
  });
  window.matchMedia = ((query: string) => ({
    matches: query.includes("prefers-reduced-motion") && prefs.reducedMotion,
    addEventListener() {},
    removeEventListener() {},
  })) as unknown as typeof window.matchMedia;
});

beforeEach(() => {
  vi.useFakeTimers();
  prefs.reduceEffects = false;
  prefs.reducedMotion = false;
  setSeen(true);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  setSeen(true);
});

/** The window is visible and focused, or not. */
function setSeen(seen: boolean) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => (seen ? "visible" : "hidden"),
  });
  vi.spyOn(document, "hasFocus").mockReturnValue(seen);
}

function item(n: number): Item {
  return {
    kind: "playlist",
    id: String(n),
    name: `Card ${n}`,
    subtitle: "",
    caption: "",
    url: "",
    color: "",
    explicit: false,
    art: { small: "", medium: "", large: "" },
    wide: { small: "", medium: `/art/0123456789abcdef0123456789abcd${n}/2400x600.jpg`, large: "" },
  } as unknown as Item;
}

const cards = Array.from({ length: 8 }, (_, i) => item(i + 1));

function renderFront(items = cards) {
  render(<FrontPage items={items} />, { wrapper: I18nRoot });
  return screen.getByRole("region", { name: "Featured" });
}

/** The lead's title: the one link (the card on its way out is hidden from queries). */
const leadName = (featured: HTMLElement) => within(featured).getByRole("link").textContent;

const tick = (ms: number) => act(() => void vi.advanceTimersByTime(ms));

describe("the lead's rotation", () => {
  it("moves to the next card every interval, wraps round, and marks the lead in the list", () => {
    const featured = renderFront();
    expect(leadName(featured)).toBe("Card 1");
    tick(ROTATE_MS - 1);
    expect(leadName(featured)).toBe("Card 1");
    tick(1);
    expect(leadName(featured)).toBe("Card 2");
    expect(
      within(featured).getByRole("button", { name: "Card 2" }).getAttribute("aria-current"),
    ).toBe("true");
    expect(
      within(featured).getByRole("button", { name: "Card 1" }).getAttribute("aria-current"),
    ).toBe(null);
    for (let i = 0; i < 7; i++) tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 1");
  });

  it("brings the list to the page the lead is on", () => {
    const featured = renderFront();
    expect(featured.textContent).toContain("1 of 2");
    for (let i = 0; i < 6; i++) tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 7");
    expect(featured.textContent).toContain("2 of 2");
    expect(within(featured).getByRole("button", { name: "Card 7" })).toBeTruthy();
  });

  it("keeps the old card, out of the tab order and the tree, only until its fade ends", () => {
    const featured = renderFront();
    tick(ROTATE_MS);
    const leaving = featured.querySelector(".lead-out") as HTMLElement;
    expect(leaving.textContent).toContain("Card 1");
    expect(leaving.hasAttribute("inert")).toBe(true);
    expect(leaving.getAttribute("aria-hidden")).toBe("true");
    expect(featured.querySelector(".lead-in")?.textContent).toContain("Card 2");
    // React listens to the prefixed name where the style object only has the prefixed property (jsdom).
    fireEvent.animationEnd(leaving);
    fireEvent(leaving, new Event("webkitAnimationEnd", { bubbles: true }));
    expect(featured.querySelector(".lead-out")).toBeNull();
    expect(leadName(featured)).toBe("Card 2");
  });

  it("makes a chosen row the lead and gives it a whole interval", () => {
    const featured = renderFront();
    tick(ROTATE_MS - 1000);
    fireEvent.click(within(featured).getByRole("button", { name: "Card 5" }));
    expect(leadName(featured)).toBe("Card 5");
    tick(ROTATE_MS - 1);
    expect(leadName(featured)).toBe("Card 5");
    tick(1);
    expect(leadName(featured)).toBe("Card 6");
  });

  it("fetches the next card's picture while the lead is read", () => {
    const asked: string[] = [];
    vi.stubGlobal(
      "Image",
      class {
        set src(url: string) {
          asked.push(url);
        }
      },
    );
    const featured = renderFront();
    expect(asked).toEqual([expect.stringContaining("abcd2/")]);
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 2");
    expect(asked.at(-1)).toContain("abcd3/");
    vi.unstubAllGlobals();
  });

  it("holds while the pointer is over the hero and starts a whole interval after it leaves", () => {
    const featured = renderFront();
    fireEvent.pointerEnter(featured);
    tick(ROTATE_MS * 3);
    expect(leadName(featured)).toBe("Card 1");
    fireEvent.pointerLeave(featured);
    tick(ROTATE_MS - 1);
    expect(leadName(featured)).toBe("Card 1");
    tick(1);
    expect(leadName(featured)).toBe("Card 2");
  });

  it("holds while focus is anywhere inside the hero, and goes on when it leaves", () => {
    const featured = renderFront();
    const first = within(featured).getByRole("button", { name: "Card 3" });
    const second = within(featured).getByRole("button", { name: "Next in Featured" });
    fireEvent.focusIn(first);
    tick(ROTATE_MS * 2);
    expect(leadName(featured)).toBe("Card 1");
    // Moving between two controls inside is not leaving.
    fireEvent.focusOut(first, { relatedTarget: second });
    tick(ROTATE_MS * 2);
    expect(leadName(featured)).toBe("Card 1");
    fireEvent.focusOut(second, { relatedTarget: document.body });
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 2");
  });

  it("stops while the window is hidden or unfocused, and goes on when it is back", () => {
    const featured = renderFront();
    tick(ROTATE_MS - 1);
    setSeen(false);
    act(() => void document.dispatchEvent(new Event("visibilitychange")));
    tick(ROTATE_MS * 3);
    expect(leadName(featured)).toBe("Card 1");
    setSeen(true);
    act(() => void document.dispatchEvent(new Event("visibilitychange")));
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 2");

    act(() => void window.dispatchEvent(new Event("blur")));
    tick(ROTATE_MS * 3);
    expect(leadName(featured)).toBe("Card 2");
    act(() => void window.dispatchEvent(new Event("focus")));
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 3");
  });

  it("waits a whole interval more at a tick that finds a menu open", () => {
    const featured = renderFront();
    const menu = document.body.appendChild(document.createElement("div"));
    menu.setAttribute("role", "menu");
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 1");
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 1");
    menu.remove();
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 2");
  });

  it("lets motion artwork play: a video playing in the lead holds the tick", () => {
    const featured = renderFront();
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { configurable: true, value: false });
    featured.querySelector(".group\\/tile")?.appendChild(video);
    tick(ROTATE_MS * 3);
    expect(leadName(featured)).toBe("Card 1");
    Object.defineProperty(video, "paused", { configurable: true, value: true });
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 2");
  });

  it("does not run under a page made inert (a modal, Now Playing)", () => {
    const featured = renderFront();
    featured.setAttribute("inert", "");
    tick(ROTATE_MS * 3);
    expect(leadName(featured)).toBe("Card 1");
    featured.removeAttribute("inert");
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 2");
  });
});

describe("the pause control", () => {
  it("pauses and resumes the rotation, and names its state", () => {
    const featured = renderFront();
    const pause = within(featured).getByRole("button", { name: "Pause featured rotation" });
    fireEvent.click(pause);
    expect(pause.getAttribute("aria-label")).toBe("Play featured rotation");
    tick(ROTATE_MS * 3);
    expect(leadName(featured)).toBe("Card 1");
    fireEvent.click(pause);
    expect(pause.getAttribute("aria-label")).toBe("Pause featured rotation");
    tick(ROTATE_MS);
    expect(leadName(featured)).toBe("Card 2");
  });

  it("says only what the user did: the rotation's own changes stay silent", () => {
    const featured = renderFront();
    const live = featured.querySelector("[aria-live]") as HTMLElement;
    tick(ROTATE_MS * 3);
    expect(live.textContent).toBe("");
    const pause = within(featured).getByRole("button", { name: "Pause featured rotation" });
    fireEvent.click(pause);
    expect(live.textContent).toBe("Rotation paused");
    fireEvent.click(pause);
    expect(live.textContent).toBe("Rotation playing");
    fireEvent.click(within(featured).getByRole("button", { name: "Next in Featured" }));
    expect(live.textContent).toBe("2 of 2");
    expect(featured.querySelectorAll("[aria-live]")).toHaveLength(1);
  });

  it("stays out of the way of a single card", () => {
    const featured = renderFront([item(1)]);
    tick(ROTATE_MS * 2);
    expect(leadName(featured)).toBe("Card 1");
    expect(within(featured).queryByRole("button", { name: /rotation/ })).toBeNull();
  });
});

describe("reduced motion", () => {
  it.each([
    ["Reduce effects", () => (prefs.reduceEffects = true)],
    ["the system's reduced motion", () => (prefs.reducedMotion = true)],
  ])("keeps the lead still under %s, with the list to change it", (_, set) => {
    set();
    const featured = renderFront();
    tick(ROTATE_MS * 3);
    expect(leadName(featured)).toBe("Card 1");
    expect(within(featured).queryByRole("button", { name: /rotation/ })).toBeNull();
    fireEvent.click(within(featured).getByRole("button", { name: "Card 4" }));
    expect(leadName(featured)).toBe("Card 4");
    tick(ROTATE_MS * 3);
    expect(leadName(featured)).toBe("Card 4");
  });
});
