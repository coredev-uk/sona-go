import { useSyncExternalStore } from "react";

const root = () => document.documentElement;

function subscribe(onChange: () => void): () => void {
  const observer = new MutationObserver(onChange);
  observer.observe(root(), { attributes: true, attributeFilter: ["class"] });
  return () => observer.disconnect();
}

/**
 * Whether the window shows the dark theme (`.dark` on `<html>`, set by the
 * appearance store), followed as it changes: the album's colours are
 * worked out per theme.
 */
export function useDark(): boolean {
  return useSyncExternalStore(subscribe, () => root().classList.contains("dark"));
}
