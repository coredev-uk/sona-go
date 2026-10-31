import { Trans, useLingui } from "@lingui/react/macro";
import { HatGlasses } from "lucide-react";

import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { isForced, setPrivate, usePrivate } from "./state";

/**
 * The player bar's private listening switch. Off: a muted glyph like its
 * neighbours. On: the glyph and "Private" in ink on the selected-control
 * tone (never amber, which means "playing"), so it reads at a glance; the
 * word drops below 1100px and the development-build note below 1280px,
 * where the tone alone carries the state. Forced on by SONA_PRIVATE=1 it
 * stays focusable (aria-disabled) so its tooltip can say why.
 */
export function PrivateButton() {
  const { t } = useLingui();
  const st = usePrivate((s) => s.state);
  const on = st?.on ?? false;
  const forced = isForced(st);
  const dev = st?.source === "development";

  // Accessible name: what the switch is and where an unchosen state comes
  // from; aria-pressed carries on/off.
  const name = forced
    ? t({
        message: "Private listening (forced by SONA_PRIVATE=1)",
        comment:
          "No Apple Music web string; Apple's apps offer the inverse, 'Use Listening History'. SONA_PRIVATE=1 is an environment variable, keep as is.",
      })
    : dev
      ? t({
          message: "Private listening (development build)",
          comment: "A development build of Sona, not a release",
        })
      : t({
          message: "Private listening",
          comment:
            "Switch name. No Apple Music web string; Apple's apps offer the inverse, 'Use Listening History'.",
        });
  const tip = forced
    ? t`On because Sona was started with SONA_PRIVATE=1`
    : on
      ? dev
        ? t`Private listening is on for this development build. Turning it off lasts until Sona quits.`
        : t`Private listening is on: plays stay out of your listening history and recommendations, and aren't scrobbled, shown on Discord or added to History.`
      : t`Turn on private listening`;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          aria-label={name}
          aria-pressed={on}
          aria-disabled={forced || !st || undefined}
          onClick={() => {
            if (st && !forced) void setPrivate(!on);
          }}
          className={cn(
            "h-7 min-w-7 gap-1 px-1.5 text-ink-muted hover:text-ink",
            on && "bg-active text-ink hover:bg-active",
            !st && "opacity-40",
          )}
        >
          <HatGlasses strokeWidth={1.75} />
          {on && (
            <span className="hidden text-meta font-medium min-[1100px]:inline" aria-hidden>
              {dev ? (
                <>
                  <span className="xl:hidden">
                    <Trans comment="Short label of the private listening switch while it is on">
                      Private
                    </Trans>
                  </span>
                  <span className="hidden xl:inline">
                    <Trans comment="Label of the private listening switch while a development build of Sona keeps it on">
                      Private (development build)
                    </Trans>
                  </span>
                </>
              ) : (
                <Trans comment="Short label of the private listening switch while it is on">
                  Private
                </Trans>
              )}
            </span>
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-72">{tip}</TooltipContent>
    </Tooltip>
  );
}
