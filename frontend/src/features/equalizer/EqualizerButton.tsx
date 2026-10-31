import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { SlidersVertical } from "lucide-react";

import { useSettings } from "@/features/settings";
import { useCapabilities } from "@/shared/lib/capabilities";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

/**
 * The player bar's way to the equaliser; a dot under the glyph while it is
 * on. Absent where the platform's playback has no equaliser.
 */
export function EqualizerButton() {
  const { t } = useLingui();
  const on = useSettings().data?.audio.equalizer.enabled ?? false;
  const supported = useCapabilities()?.playback.equalizer ?? false;
  const label = on ? t`Equaliser (on)` : t`Equaliser`;
  if (!supported) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          asChild
          variant="ghost"
          size="icon-sm"
          className={cn(
            "relative text-ink-muted hover:text-ink",
            on &&
              "text-ink after:absolute after:bottom-0.5 after:left-1/2 after:size-1 after:-translate-x-1/2 after:rounded-full after:bg-ink",
          )}
        >
          <Link to="/equalizer" aria-label={label}>
            <SlidersVertical strokeWidth={1.75} />
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
