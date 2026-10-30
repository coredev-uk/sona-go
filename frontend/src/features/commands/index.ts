import { t } from "@lingui/core/macro";
import { Command, Keyboard } from "lucide-react";

import type { FeatureModule } from "@/shared/module";

import { CommandsHost } from "./CommandsHost";
import { useCommands } from "./store";

/**
 * Commands: the Ctrl+K palette (pages, every registered shortcut, library
 * items) and the "?" sheet listing every shortcut with its current keys.
 */
export const commandsModule: FeatureModule<never> = {
  id: "commands",
  routes: [],
  shortcuts: [
    {
      id: "commands.palette",
      label: () => t`Open the command palette`,
      group: "general",
      keys: ["Ctrl+K"],
      scope: "global",
      icon: Command,
      palette: false,
      run: () => useCommands.getState().show("palette"),
    },
    {
      id: "commands.help",
      label: () => t`Keyboard shortcuts`,
      group: "general",
      keys: ["?"],
      keywords: () => t`help keys bindings`,
      icon: Keyboard,
      run: () => useCommands.getState().show("help"),
    },
  ],
  shellSlots: { Provider: CommandsHost },
};
