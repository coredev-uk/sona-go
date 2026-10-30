import { t } from "@lingui/core/macro";

import type { ShortcutGroup } from "@/shared/module";

/** The heading for a shortcut group in the palette, help sheet and Settings. */
export function groupTitle(group: ShortcutGroup): string {
  switch (group) {
    case "playback":
      return t`Playback`;
    case "view":
      return t`View`;
    case "pages":
      return t`Pages`;
    case "navigation":
      return t`Navigation`;
    case "general":
      return t`General`;
  }
}
