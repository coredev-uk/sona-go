import { RemindersService, type Album, type Reminder } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";

import { run } from "@/shared/lib/run";

/** Every release reminder set, kept current by "reminders:changed". */
export const remindersKey = ["reminders"] as const;

export function useReminders() {
  return useQuery({
    queryKey: remindersKey,
    queryFn: () => RemindersService.List(),
    staleTime: Infinity,
  });
}

/** The reminder for a catalog album, if one is set. */
export function reminderFor(list: readonly Reminder[] | undefined, album: Album) {
  return list?.find((r) => r.albumId === album.catalogId);
}

/**
 * Turns the reminder for an upcoming album on or off. The notification's
 * words are the interface's (Go has none of its own): the album's name as
 * its title, "Out now · {artist}" as its body.
 */
export async function setReminder(album: Album, on: boolean) {
  const name = album.name;
  if (!on) {
    if (await run(t`Couldn't remove the reminder`, RemindersService.Remove(album.catalogId))) {
      toast(t`You won't be reminded about “${name}”`);
    }
    return;
  }
  const artistName = album.artistName;
  const message = t`Out now · ${artistName}`;
  if (await run(t`Couldn't set the reminder`, RemindersService.Set(album.catalogId, message))) {
    toast(t`You'll be reminded when “${name}” is out`);
  }
}
