import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { bindAppearance, resolveTheme, useAppearance } from "./appearance";

type Listener = (e: MediaQueryListEvent) => void;

function fakeMatchMedia(initial: boolean) {
  const listeners = new Set<Listener>();
  const mql = {
    matches: initial,
    addEventListener: (_: string, l: Listener) => listeners.add(l),
    removeEventListener: (_: string, l: Listener) => listeners.delete(l),
  };
  vi.stubGlobal("matchMedia", () => mql);
  return {
    listeners,
    setSystemDark(dark: boolean) {
      mql.matches = dark;
      listeners.forEach((l) => l({ matches: dark } as MediaQueryListEvent));
    },
  };
}

describe("resolveTheme", () => {
  it("follows the system only for the system setting", () => {
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
    expect(resolveTheme("light", true)).toBe("light");
    expect(resolveTheme("dark", false)).toBe("dark");
  });
});

describe("bindAppearance", () => {
  let root: HTMLElement;

  beforeEach(() => {
    root = document.createElement("html");
    useAppearance.getState().load({ theme: "system", reduceEffects: false });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("tracks the system scheme while the theme is system", () => {
    const media = fakeMatchMedia(false);
    const unbind = bindAppearance(root);
    expect(root.classList.contains("dark")).toBe(false);

    media.setSystemDark(true);
    expect(root.classList.contains("dark")).toBe(true);

    useAppearance.getState().setTheme("light");
    expect(root.classList.contains("dark")).toBe(false);
    media.setSystemDark(true);
    expect(root.classList.contains("dark")).toBe(false);

    unbind();
    expect(media.listeners.size).toBe(0);
  });

  it("toggles reduce-effects and stops updating after unbind", () => {
    fakeMatchMedia(true);
    const unbind = bindAppearance(root);
    useAppearance.getState().setReduceEffects(true);
    expect(root.classList.contains("reduce-effects")).toBe(true);
    expect(root.classList.contains("dark")).toBe(true);

    unbind();
    useAppearance.getState().setReduceEffects(false);
    expect(root.classList.contains("reduce-effects")).toBe(true);
  });
});
