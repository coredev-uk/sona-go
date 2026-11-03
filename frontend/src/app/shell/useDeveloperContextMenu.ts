import { useEffect } from "react";

import { useCapabilities } from "@/shared/lib/capabilities";

/** The Wails runtime's switch for the web view's own context menu (its CSS variable). */
const MENU_VARIABLE = "--default-contextmenu";

/**
 * While the web inspector is on (Capabilities.developerTools), a right click
 * that no Sona menu handles opens the web view's own menu, whose Inspect
 * Element opens the inspector. Wails shows that menu everywhere in a
 * development build; a release build only over selected text and inputs,
 * unless this says `show`.
 */
export function useDeveloperContextMenu() {
  const on = useCapabilities()?.developerTools ?? false;
  useEffect(() => {
    if (!on) return;
    const root = document.documentElement;
    root.style.setProperty(MENU_VARIABLE, "show");
    return () => {
      root.style.removeProperty(MENU_VARIABLE);
    };
  }, [on]);
}
