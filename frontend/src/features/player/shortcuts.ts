import { PlaybackService } from "@bindings/services";

import { run } from "@/shared/lib/run";
import type { Shortcut } from "@/shared/module";

import { useUI } from "./ui";

/** Targets whose own keyboard handling must win over the player's. */
const interactive =
  "input, textarea, select, button, a[href], [contenteditable], [role=slider], [role=menuitem], [role=option], [role=combobox], [role=tab], [role=switch]";

/**
 * Player shortcuts, media-key style: Space plays or pauses when nothing
 * that takes Space has focus, Ctrl+→ / Ctrl+← skip, Escape closes the
 * now-playing view. "/" and Ctrl+F (search) live in the top bar.
 */
export const playerShortcuts: Shortcut[] = [
  {
    id: "player.toggle",
    label: "Play or pause",
    matches: (e, { target }) =>
      e.key === " " && !e.ctrlKey && !e.shiftKey && !target?.closest(interactive),
    run: () => void run("Couldn't play or pause", PlaybackService.Toggle()),
  },
  {
    id: "player.skip",
    label: "Next or previous track",
    matches: (e, { typing }) =>
      e.ctrlKey && !typing && (e.key === "ArrowRight" || e.key === "ArrowLeft"),
    run: (e) =>
      void run(
        "Couldn't skip",
        e.key === "ArrowRight" ? PlaybackService.Next() : PlaybackService.Prev(),
      ),
  },
  {
    id: "player.close-overlay",
    label: "Close the now-playing view",
    matches: (e) => e.key === "Escape" && useUI.getState().overlay !== "none",
    run: () => useUI.getState().closeOverlay(),
  },
];
