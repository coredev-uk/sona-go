import { Moon } from "lucide-react";
import { t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { Fragment, useEffect, useState } from "react";

import { clockTime } from "@/shared/lib/format";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { setSleep } from "./commands";
import { usePlayback } from "./playback";

/** The timer choices: minutes, or the end of the song or of the queue. */
const choices = [
  { value: "15", label: () => t`In 15 minutes` },
  { value: "30", label: () => t`In 30 minutes` },
  { value: "45", label: () => t`In 45 minutes` },
  { value: "60", label: () => t`In 1 hour` },
  { value: "120", label: () => t`In 2 hours` },
  { value: "track", label: () => t`At the end of this song` },
  { value: "queue", label: () => t`At the end of the queue` },
] as const;

/** Minutes left (rounded up) until `endsAt`, ticking while the timer runs. */
function useMinutesLeft(endsAt: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!endsAt) return;
    // Tick on the minute boundary of the countdown, not every second.
    let timer = 0;
    const tick = () => {
      const t = Date.now();
      setNow(t);
      if (t < endsAt) timer = window.setTimeout(tick, ((endsAt - t) % 60_000) + 50);
    };
    tick();
    return () => window.clearTimeout(timer);
  }, [endsAt]);
  return Math.max(Math.ceil((endsAt - now) / 60_000), 0);
}

/**
 * The sleep timer in the player bar: pauses playback after a while, or at
 * the end of the song or the queue, fading out over the last 10 seconds.
 * While it runs the moon shows how many minutes are left.
 */
export function SleepTimer({ disabled }: { disabled: boolean }) {
  const sleep = usePlayback((s) => s.sleep);
  const minutes = useMinutesLeft(sleep?.mode === "after" ? sleep.endsAt : 0);
  const [chosen, setChosen] = useState("");
  const { t } = useLingui();

  // The radio value: the minutes the user picked while the timer they set runs.
  const value = !sleep ? "" : sleep.mode === "after" ? chosen : sleep.mode;
  const until = sleep?.mode === "after" ? clockTime(sleep.endsAt) : "";
  const status = !sleep
    ? t`Sleep timer`
    : sleep.mode === "after"
      ? t`Sleep timer: ${minutes}m left, pauses at ${until}`
      : sleep.mode === "track"
        ? t`Sleep timer: pauses after this song`
        : t`Sleep timer: pauses after the queue`;

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              aria-label={status}
              disabled={disabled && !sleep}
              className={cn(
                "h-7 min-w-7 gap-1 px-1.5 text-ink-muted hover:text-ink disabled:opacity-40",
                sleep && "text-ink",
              )}
            >
              <Moon strokeWidth={1.75} fill={sleep ? "currentColor" : "none"} />
              {sleep?.mode === "after" && (
                <span className="text-meta num" aria-hidden>
                  <Trans>{minutes}m</Trans>
                </span>
              )}
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{status}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end" side="top" className="w-60">
        <DropdownMenuLabel className="text-meta font-normal text-ink-faint">
          <Trans>Stop playing</Trans>
        </DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={value}
          onValueChange={(v) => {
            setChosen(v);
            if (v === "track" || v === "queue") setSleep(v);
            else setSleep("after", Number(v));
          }}
        >
          {choices.map((c, i) => (
            <Fragment key={c.value}>
              {i === 5 && <DropdownMenuSeparator />}
              <DropdownMenuRadioItem value={c.value}>{c.label()}</DropdownMenuRadioItem>
            </Fragment>
          ))}
        </DropdownMenuRadioGroup>
        {sleep && (
          <>
            <DropdownMenuSeparator />
            <p className="px-2 py-1.5 text-meta text-ink-muted">
              {sleep.mode === "after" ? (
                <Trans>
                  Pauses at <span className="num">{until}</span>, fading out over the last 10
                  seconds.
                </Trans>
              ) : sleep.mode === "track" ? (
                <Trans>Pauses when this song ends, fading out over the last 10 seconds.</Trans>
              ) : (
                <Trans>
                  Pauses when the last song you queued ends, fading out over the last 10 seconds.
                </Trans>
              )}
            </p>
            <DropdownMenuItem onSelect={() => setSleep("")}>
              <Trans>Turn off sleep timer</Trans>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
