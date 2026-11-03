import type { EditorialPage, Item, Section } from "@bindings/services";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { NewPage } from "./NewPage";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a href="/" className={className}>
      {children}
    </a>
  ),
}));
vi.mock("@/shared/items/ItemMenu", () => ({ ItemMenu: () => null }));
vi.mock("@/shared/items/play", () => ({ canPlay: () => false, playItem: vi.fn() }));
vi.mock("@/shared/items/cover", () => ({ openExternal: vi.fn() }));
vi.mock("@/features/settings", () => ({
  useAppearance: (select: (s: { reduceEffects: boolean }) => boolean) =>
    select({ reduceEffects: false }),
}));
vi.mock("@/shared/items/EditorialSections", () => ({
  EditorialSections: ({ sections }: { sections: Section[] }) =>
    sections.map((s) => (
      <section key={s.id} aria-label={s.title}>
        <h2>{s.title}</h2>
        <ul>
          {s.items.map((i) => (
            <li key={i.id}>{i.name}</li>
          ))}
        </ul>
      </section>
    )),
}));

const page = vi.hoisted(() => ({ data: undefined as EditorialPage | undefined }));
vi.mock("./queries", () => ({
  useEditorialPage: () => ({ data: page.data, isPending: false, isError: false }),
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
  Element.prototype.scrollIntoView = vi.fn();
  window.matchMedia = (() => ({
    matches: false,
    addEventListener() {},
    removeEventListener() {},
  })) as unknown as typeof window.matchMedia;
});
afterEach(cleanup);

const banner = (n: number) =>
  `/art/0123456789abcdef0123456789abcd${String(n).padStart(2, "0")}/2400x600.jpg`;

function item(n: number, kind = "playlist"): Item {
  return {
    kind,
    id: String(n),
    name: `Card ${n}`,
    subtitle: "",
    caption: "",
    url: "",
    color: "",
    explicit: false,
    art: { small: "", medium: "", large: "" },
    wide: { small: "", medium: banner(n), large: banner(n) },
  } as unknown as Item;
}

function section(id: string, kind: string, items: Item[], title = id): Section {
  return { id, kind, title, text: "", items, room: "", seeAll: false, source: "" };
}

function renderPage(sections: Section[]) {
  page.data = { sections, fetchedAt: Date.now(), stale: false } as unknown as EditorialPage;
  return render(<NewPage />, { wrapper: I18nRoot });
}

const hero = section(
  "hero",
  "hero",
  Array.from({ length: 16 }, (_, i) => item(i + 1)),
  "",
);

describe("Discover's front page", () => {
  it("leads with the first featured card, at the lead's width, and lists them all beside it a page at a time", () => {
    renderPage([hero]);
    const featured = screen.getByRole("region", { name: "Featured" });
    // Two thirds of 1200px less the gap, at Apple's 1060:608 hero shape.
    const lead = featured.querySelector("img");
    expect(lead?.getAttribute("src")).toMatch(/\/784x450\.jpg$/);
    const list = within(featured).getByRole("list", { name: "Featured cards" });
    // As many rows as fit beside the lead: six of the sixteen, the lead's first.
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual(["Card 1", "Card 2", "Card 3", "Card 4", "Card 5", "Card 6"]);
    expect(within(list).getByRole("button", { name: "Card 1" }).getAttribute("aria-current")).toBe(
      "true",
    );
    expect(featured.textContent).toContain("1 of 3");
  });

  it("pages the list round, so the pager never disables the focused button", () => {
    renderPage([hero]);
    const featured = screen.getByRole("region", { name: "Featured" });
    const previous = within(featured).getByRole("button", { name: "Previous in Featured" });
    fireEvent.click(previous);
    expect(featured.textContent).toContain("3 of 3");
    expect(
      within(featured)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual(["Card 13", "Card 14", "Card 15", "Card 16"]);
    expect((previous as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(within(featured).getByRole("button", { name: "Next in Featured" }));
    expect(featured.textContent).toContain("1 of 3");
  });
});

describe("Discover's contents", () => {
  const sections = [
    hero,
    section("Recent Releases", "shelf", [item(20, "album")]),
    section("Updated Playlists", "shelf", [item(21)]),
    section("Live Radio", "live", [item(22, "station")]),
  ];

  it("lists the bands in order with their shelves, and sets the shelves under their band", () => {
    renderPage(sections);
    const contents = screen.getByRole("navigation", { name: "On this page" });
    expect(
      within(contents)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual([
      "Songs and albums",
      "Recent Releases",
      "Playlists",
      "Updated Playlists",
      "Radio",
      "Live Radio",
    ]);
    const radio = screen.getByRole("region", { name: "Radio" });
    expect(within(radio).getByRole("heading", { name: "Live Radio" })).toBeTruthy();
  });

  it("jumps to a shelf and moves focus to it", () => {
    renderPage(sections);
    const contents = screen.getByRole("navigation", { name: "On this page" });
    fireEvent.click(within(contents).getByRole("button", { name: "Updated Playlists" }));
    const shelf = screen.getByRole("region", { name: "Updated Playlists" }).parentElement;
    expect(document.activeElement).toBe(shelf);
    expect(shelf?.scrollIntoView).toHaveBeenCalled();
  });
});

describe("Discover's shelves", () => {
  it("leave out what the front page already shows, unless that would empty the shelf", () => {
    renderPage([
      hero,
      section("New This Week", "shelf", [item(3), item(30, "album")]),
      section("Spotlight", "shelf", [item(4)]),
    ]);
    const names = (name: string) =>
      within(screen.getByRole("region", { name }))
        .getAllByRole("listitem")
        .map((li) => li.textContent);
    expect(names("New This Week")).toEqual(["Card 30"]);
    expect(names("Spotlight")).toEqual(["Card 4"]);
  });
});
