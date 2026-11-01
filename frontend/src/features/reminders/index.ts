import { RemindersService, type Reminder } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { Bell } from "lucide-react";

import type { ActionDef, MenuData } from "@/shared/actions/registry";
import { currentCapabilities } from "@/shared/lib/capabilities";
import { isUpcoming } from "@/shared/lib/release";
import type { FeatureModule } from "@/shared/module";

import { reminderFor, remindersKey, setReminder } from "./queries";

export { RemindButton } from "./RemindButton";

/** The reminders, loaded when an album's menu opens. */
const reminders: MenuData<"album", Reminder[]> = {
  id: "reminders.list",
  load: () => RemindersService.List().catch(() => []),
};

const remindAction: ActionDef<"album"> = {
  id: "reminders.toggle",
  target: "album",
  label: (album, ctx) =>
    reminderFor(ctx.data(reminders), album) ? t`Turn off reminder` : t`Remind me`,
  icon: Bell,
  group: "item",
  order: 25,
  data: [reminders],
  isAvailable: (album) =>
    isUpcoming(album) &&
    album.releaseAt > 0 &&
    !!album.catalogId &&
    !!currentCapabilities()?.notifications,
  isEnabled: (_, ctx) => ctx.data(reminders) !== undefined,
  run: (album, ctx) => void setReminder(album, !reminderFor(ctx.data(reminders), album)),
};

/**
 * Release reminders (Go's RemindersService): an upcoming album's "Remind
 * me" on its page and in its menus, for one desktop notification when it
 * comes out. The list mirrors "reminders:changed".
 */
export const remindersModule: FeatureModule<never> = {
  id: "reminders",
  routes: [],
  actions: [remindAction],
  eventHandlers: {
    "reminders:changed": (data, { qc }) => qc.setQueryData(remindersKey, data),
  },
};
