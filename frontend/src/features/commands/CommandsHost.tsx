import type { ReactNode } from "react";

import { CommandPalette } from "./CommandPalette";
import { ShortcutHelp } from "./ShortcutHelp";

/** The shell Provider slot: mounts the palette and the help sheet once. */
export function CommandsHost({ children }: { children: ReactNode }) {
  return (
    <>
      {children}
      <CommandPalette />
      <ShortcutHelp />
    </>
  );
}
