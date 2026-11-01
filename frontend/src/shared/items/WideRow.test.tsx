import type { Item } from "@bindings/services";
import { cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { I18nRoot } from "@/shared/i18n";

import { WideRow } from "./WideRow";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/">{children}</a>,
}));
vi.mock("./ItemMenu", () => ({ ItemMenu: () => null }));
vi.mock("./play", () => ({ canPlay: () => false, playItem: vi.fn() }));
vi.mock("./cover", () => ({ openExternal: vi.fn() }));

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});
afterEach(cleanup);

const art = (w: number, h: number) => `/art/0123456789abcdef0123456789abcdef/${w}x${h}.jpg`;

function item(id: string, wide: string, cover = ""): Item {
  return {
    kind: "album",
    id,
    name: `Item ${id}`,
    subtitle: "",
    caption: "",
    url: "",
    color: "#102030",
    explicit: false,
    art: { small: "", medium: cover, large: cover },
    wide: { small: "", medium: wide, large: wide },
  } as unknown as Item;
}

function frames(items: Item[], ratio: number, crop = false) {
  const { container } = render(
    <WideRow title="Row" items={items} min={240} ratio={ratio} crop={crop} />,
    { wrapper: I18nRoot },
  );
  const found = container.querySelectorAll<HTMLElement>("[role=listitem] [style*='aspect-ratio']");
  return [...found].map((f) => ({
    // jsdom serialises aspect-ratio as "<w> / <h>".
    ratio: f.style.aspectRatio
      .split("/")
      .map(Number)
      .reduce((w, h) => w / h),
    src: f.querySelector("img")?.getAttribute("src") ?? "",
    cover: f.querySelector("img")?.classList.contains("object-cover") ?? false,
  }));
}

describe("WideRow", () => {
  it("frames a row of 4:1 banners at 4:1, requested at that shape", () => {
    const [f] = frames([item("1", art(1000, 250)), item("2", art(1000, 250))], 2);
    expect(f.ratio).toBe(4);
    expect(f.src).toMatch(/\/240x60\.jpg$/);
  });

  it("keeps the tallest banner's frame in a mixed row and letterboxes the wider one", () => {
    const fs = frames([item("1", art(1000, 250)), item("2", art(1000, 417))], 2);
    expect(fs[0].ratio).toBeCloseTo(2.4, 2);
    // The 4:1 banner is bound by the frame's width, not squeezed or cropped.
    expect(fs[0].src).toMatch(/\/240x60\.jpg$/);
  });

  it("never frames taller than its ratio, even for a square banner", () => {
    const [f] = frames([item("1", art(400, 400))], 2.4);
    expect(f.ratio).toBe(2.4);
    expect(f.src).toMatch(/\/100x100\.jpg$/);
  });

  it("shows a card without a banner by its whole cover inside the frame", () => {
    const [f] = frames([item("1", "", art(1000, 250))], 2.4);
    expect(f.ratio).toBe(2.4);
    expect(f.src).toMatch(/\/240x60\.jpg$/);
  });

  it("crops every banner to a cropping row's frame and keeps covers whole", () => {
    const [wide] = frames([item("1", art(1000, 250))], 1.75, true);
    const [hero] = frames([item("2", art(1000, 417))], 1.75, true);
    const [cover] = frames([item("3", "", art(400, 400))], 1.75, true);
    // Banners are requested at the frame's shape: Apple crops them to it.
    expect(wide).toMatchObject({
      ratio: 1.75,
      cover: true,
      src: expect.stringMatching(/\/240x137\.jpg$/),
    });
    expect(hero).toMatchObject({
      ratio: 1.75,
      cover: true,
      src: expect.stringMatching(/\/240x137\.jpg$/),
    });
    expect(cover).toMatchObject({
      ratio: 1.75,
      cover: false,
      src: expect.stringMatching(/\/137x137\.jpg$/),
    });
  });
});
