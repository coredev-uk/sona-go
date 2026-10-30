import { create } from "zustand";

/** Theme setting, mirroring Go's config.Settings.Theme. */
export type Theme = "system" | "light" | "dark";

export interface Appearance {
  theme: Theme;
  /** Solid surfaces, no backdrops, no motion (Settings → Appearance). */
  reduceEffects: boolean;
}

const darkQuery = "(prefers-color-scheme: dark)";

/** Resolves a theme setting against the system preference. */
export function resolveTheme(theme: Theme, systemPrefersDark: boolean): "light" | "dark" {
  if (theme === "system") return systemPrefersDark ? "dark" : "light";
  return theme;
}

/**
 * Applies appearance to the root element: `.dark` for the resolved theme and
 * `.reduce-effects` for the solid-surface mode. Idempotent.
 */
export function applyAppearance(
  root: HTMLElement,
  appearance: Appearance,
  systemPrefersDark: boolean,
): void {
  const dark = resolveTheme(appearance.theme, systemPrefersDark) === "dark";
  root.classList.toggle("dark", dark);
  root.classList.toggle("reduce-effects", appearance.reduceEffects);
}

interface AppearanceStore extends Appearance {
  setTheme: (theme: Theme) => void;
  setReduceEffects: (on: boolean) => void;
  /** Replaces both values, e.g. from settings loaded in Go. */
  load: (appearance: Appearance) => void;
}

export const useAppearance = create<AppearanceStore>((set) => ({
  theme: "system",
  reduceEffects: false,
  setTheme: (theme) => set({ theme }),
  setReduceEffects: (reduceEffects) => set({ reduceEffects }),
  load: ({ theme, reduceEffects }) => set({ theme, reduceEffects }),
}));

/**
 * Keeps the document's classes in sync with the store and with the system
 * colour scheme. Returns an unsubscribe function.
 */
export function bindAppearance(root: HTMLElement = document.documentElement): () => void {
  const media = window.matchMedia(darkQuery);
  const apply = () => applyAppearance(root, useAppearance.getState(), media.matches);
  apply();
  const unsubscribe = useAppearance.subscribe(apply);
  media.addEventListener("change", apply);
  return () => {
    unsubscribe();
    media.removeEventListener("change", apply);
  };
}
