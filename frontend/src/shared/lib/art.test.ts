import { afterEach, describe, expect, it } from "vitest";

import { artAspect, artUrl } from "./art";

const hero = "/art/0123456789abcdef0123456789abcdef/1000x417.jpg";
const brick = "/art/0123456789abcdef0123456789abcdef/1000x250.jpg";
const cover = "/art/0123456789abcdef0123456789abcdef/400x400.jpg";

function density(ratio: number) {
  Object.defineProperty(window, "devicePixelRatio", { value: ratio, configurable: true });
}

afterEach(() => density(1));

describe("artAspect", () => {
  it("reads the shape from the URL, square for the old <n>.jpg form", () => {
    expect(artAspect(hero)).toBeCloseTo(2.4, 2);
    expect(artAspect(brick)).toBe(4);
    expect(artAspect("/art/k/96.jpg")).toBe(1);
    expect(artAspect("/art/k/backdrop.jpg")).toBe(1);
    expect(artAspect(undefined)).toBe(1);
  });
});

describe("artUrl", () => {
  it("asks for the rendered box at the screen's density", () => {
    density(2);
    expect(artUrl(cover, { width: 48, height: 48 })).toBe(
      "/art/0123456789abcdef0123456789abcdef/96x96.jpg",
    );
    expect(artUrl(cover, { width: 240, height: 135 })).toMatch(/\/480x270\.jpg$/);
  });

  it("caps the density at 2 and never goes below 1", () => {
    density(3);
    expect(artUrl(cover, { width: 100, height: 100 })).toMatch(/\/200x200\.jpg$/);
    density(0.5);
    expect(artUrl(cover, { width: 100, height: 100 })).toMatch(/\/100x100\.jpg$/);
  });

  it("fits wide art inside the box at its own shape for contain", () => {
    density(2);
    // A 2.4:1 banner fills a 2.4:1 frame.
    expect(artUrl(hero, { width: 480, height: 200 }, "contain")).toMatch(/\/959x400\.jpg$/);
    // A 4:1 banner in a 2.4:1 frame is bound by the width.
    expect(artUrl(brick, { width: 480, height: 200 }, "contain")).toMatch(/\/960x240\.jpg$/);
    // A square cover in a wide frame is bound by the height.
    expect(artUrl(cover, { width: 480, height: 200 }, "contain")).toMatch(/\/400x400\.jpg$/);
  });

  it("keeps the shape while bringing the long edge into 16–2048", () => {
    density(2);
    expect(artUrl(hero, { width: 2400, height: 1000 }, "contain")).toMatch(/\/2048x854\.jpg$/);
    expect(artUrl(cover, { width: 4, height: 2 })).toMatch(/\/16x8\.jpg$/);
  });

  it("rewrites the old square URLs and keeps a retry query", () => {
    expect(artUrl("/art/k/96.jpg?retry=1", { width: 32, height: 32 })).toBe(
      "/art/k/32x32.jpg?retry=1",
    );
    expect(artUrl(undefined, { width: 32, height: 32 })).toBeUndefined();
  });
});
