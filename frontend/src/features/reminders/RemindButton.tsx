import type { Album } from "@bindings/services";
import { Trans } from "@lingui/react/macro";
import { Bell, BellRing } from "lucide-react";

import { useCapabilities } from "@/shared/lib/capabilities";
import { fullReleaseDate } from "@/shared/lib/release";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { reminderFor, setReminder, useReminders } from "./queries";

/**
 * "Remind me" on an upcoming album's page: a toggle (its name stays the
 * same; pressed means a reminder is set) for one desktop notification when
 * the album comes out. Hidden where the platform has no notifications and
 * for albums without a full release date or a catalog ID.
 */
export function RemindButton({ album }: { album: Album }) {
  const caps = useCapabilities();
  const reminders = useReminders();
  if (!caps?.notifications || !album.catalogId || album.releaseAt === 0) return null;
  const on = !!reminderFor(reminders.data, album);
  const releaseDate = fullReleaseDate(album.releaseAt);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          aria-pressed={on}
          disabled={reminders.isPending}
          onClick={() => void setReminder(album, !on)}
          className="aria-pressed:bg-active"
        >
          {on ? <BellRing strokeWidth={1.75} /> : <Bell strokeWidth={1.75} />}
          <Trans>Remind me</Trans>
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {on ? (
          <Trans>You'll get a notification on {releaseDate}</Trans>
        ) : (
          <Trans>Get a notification when it's out</Trans>
        )}
      </TooltipContent>
    </Tooltip>
  );
}
