import { useEffect } from "react";

import type { Shortcut } from "@/shared/module";

/**
 * One window keydown listener for every module's shortcuts: keys already
 * handled, with Alt or Meta, or inside an open menu, dialog or listbox are
 * left alone; otherwise the first shortcut that matches takes the key.
 */
export function useShortcuts(list: readonly Shortcut[]) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.altKey || e.metaKey) return;
      const target = e.target instanceof HTMLElement ? e.target : null;
      // Inside an open menu or dialog, keys belong to it.
      if (target?.closest("[role=menu], [role=dialog], [role=listbox]")) return;
      const typing = !!target?.closest("input, textarea, select, [contenteditable]");
      const shortcut = list.find((s) => s.matches(e, { target, typing }));
      if (!shortcut) return;
      e.preventDefault();
      shortcut.run(e);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [list]);
}
