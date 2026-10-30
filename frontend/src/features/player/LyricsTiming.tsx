import { Minus, Plus } from "lucide-react";
import { Trans, useLingui } from "@lingui/react/macro";
import type { ReactNode } from "react";

import { numberFormat } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";
import { useBinding } from "@/shared/shortcuts/bindings";
import { ChordKeys } from "@/shared/shortcuts/Keys";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { describeOffset, OFFSET_STEP_MS, useLyricsOffset, useSongOffset } from "./lyrics";

function TimingButton({
  label,
  shortcut,
  disabled,
  onClick,
  className,
  children,
}: {
  label: string;
  shortcut: string;
  disabled?: boolean;
  onClick: () => void;
  className?: string;
  children: ReactNode;
}) {
  const keys = useBinding(shortcut);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          disabled={disabled}
          onClick={onClick}
          className={cn("text-ink-muted hover:text-ink", className)}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {label}
        {keys[0] && <ChordKeys chord={keys[0]} className="ml-1.5" />}
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * The lyrics timing nudge for one song: earlier and later by half a
 * second, and the offset in words between them (pressing it puts the
 * lyrics back on Apple's timing). Kept per song for the session.
 */
export function LyricsTiming({ songId, className }: { songId: string; className?: string }) {
  const offset = useSongOffset(songId);
  const nudge = useLyricsOffset((s) => s.nudge);
  const reset = useLyricsOffset((s) => s.reset);
  const resetKeys = useBinding("player.lyrics-reset");
  const words = describeOffset(offset);
  const { t } = useLingui();
  const step = numberFormat().format(OFFSET_STEP_MS / 1000);
  return (
    <div role="group" aria-label={t`Lyrics timing`} className={cn("flex items-center", className)}>
      <TimingButton
        label={t`Show lyrics ${step} s earlier`}
        shortcut="player.lyrics-earlier"
        onClick={() => nudge(songId, -1)}
      >
        <Minus strokeWidth={1.75} />
      </TimingButton>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            disabled={!words}
            onClick={() => reset(songId)}
            // Announced as it changes, so a nudge from the keyboard is heard.
            aria-live="polite"
            className="h-7 min-w-[7.5rem] rounded-md px-2 text-sub text-ink-muted num transition-colors duration-100 enabled:hover:bg-hover enabled:hover:text-ink disabled:cursor-default"
          >
            {words ? t`Lyrics ${words}` : t`Lyrics on time`}
          </button>
        </TooltipTrigger>
        {words && (
          <TooltipContent>
            <Trans>Back to the original timing</Trans>
            {resetKeys[0] && <ChordKeys chord={resetKeys[0]} className="ml-1.5" />}
          </TooltipContent>
        )}
      </Tooltip>
      <TimingButton
        label={t`Show lyrics ${step} s later`}
        shortcut="player.lyrics-later"
        onClick={() => nudge(songId, 1)}
      >
        <Plus strokeWidth={1.75} />
      </TimingButton>
    </div>
  );
}
