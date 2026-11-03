import { useLingui } from "@lingui/react/macro";
import { Volume1, Volume2, VolumeX } from "lucide-react";
import { useId, type ReactNode } from "react";

import { useCapabilities } from "@/shared/lib/capabilities";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Slider } from "@/shared/ui/slider";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { setVolume, toggleMute } from "./commands";
import { usePlayback } from "./playback";

/**
 * The player's quiet icon button (the bar, the video view), its label also
 * its tooltip. `disabled` is for what can't be pressed right now (nothing
 * playing, the end of the queue). `unavailable` is for a control that
 * doesn't apply to what plays (a live station) and says why: the button
 * stays focusable (aria-disabled) so its tooltip and description are
 * reached by keyboard and screen reader, and a press does nothing.
 */
export function IconButton({
  id,
  label,
  disabled,
  unavailable,
  pressed,
  onClick,
  children,
}: {
  id?: string;
  label: string;
  disabled?: boolean;
  /** Why the control doesn't apply to what plays; set, the button is aria-disabled. */
  unavailable?: string;
  /** Toggle buttons (shuffle, repeat, queue, lyrics) report their state. */
  pressed?: boolean;
  onClick?: () => void;
  children: ReactNode;
}) {
  const reasonId = useId();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          id={id}
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          aria-pressed={pressed}
          aria-disabled={unavailable ? true : undefined}
          aria-describedby={unavailable ? reasonId : undefined}
          disabled={disabled}
          onClick={unavailable ? undefined : onClick}
          className={cn(
            "relative text-ink-muted hover:text-ink disabled:opacity-40",
            unavailable && "opacity-40 hover:bg-transparent hover:text-ink-muted",
            // On: ink, with a dot under the glyph (amber is kept for "playing").
            pressed &&
              "text-ink after:absolute after:bottom-0.5 after:left-1/2 after:size-1 after:-translate-x-1/2 after:rounded-full after:bg-ink",
          )}
        >
          {children}
          {unavailable && (
            <span id={reasonId} hidden>
              {unavailable}
            </span>
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {label}
        {unavailable && <span className="mt-0.5 block opacity-75">{unavailable}</span>}
      </TooltipContent>
    </Tooltip>
  );
}

/** Mute and the volume slider, where Sona sets the playback volume. */
export function Volume() {
  const volume = usePlayback((s) => s.volume);
  const supported = useCapabilities()?.playback.volume ?? false;
  const VolumeIcon = volume === 0 ? VolumeX : volume < 0.5 ? Volume1 : Volume2;
  const { t } = useLingui();
  if (!supported) return null;
  return (
    <div
      className="flex items-center gap-1"
      style={{ "--slider-range": "var(--ink-muted)" } as React.CSSProperties}
    >
      <IconButton label={volume === 0 ? t`Unmute` : t`Mute`} onClick={toggleMute}>
        <VolumeIcon strokeWidth={1.75} />
      </IconButton>
      <Slider
        aria-label={t`Volume`}
        className="w-24 shrink-0"
        value={[Math.round(volume * 100)]}
        max={100}
        step={1}
        onValueChange={([v]) => setVolume(v / 100)}
      />
    </div>
  );
}
