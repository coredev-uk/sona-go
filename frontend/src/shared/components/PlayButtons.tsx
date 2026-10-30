import { Play, Shuffle } from "lucide-react";

import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

interface PlayButtonsProps {
  /** What plays, for the accessible names: "OK Computer", "your songs". */
  label: string;
  disabled?: boolean;
  onPlay: () => void;
  /** Omitted for a single item (a song): no Shuffle button. */
  onShuffle?: () => void;
}

/**
 * The collection's play control (the amber disc: amber means playing) and
 * a quiet Shuffle beside it.
 */
export function PlayButtons({ label, disabled, onPlay, onShuffle }: PlayButtonsProps) {
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="icon"
            aria-label={`Play ${label}`}
            disabled={disabled}
            onClick={onPlay}
            className="size-10 rounded-full bg-play text-on-play hover:bg-play/90 disabled:bg-hover disabled:text-ink-faint disabled:opacity-100"
          >
            <Play fill="currentColor" strokeWidth={0} className="size-[18px] translate-x-px" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Play</TooltipContent>
      </Tooltip>
      {onShuffle && (
        <Button variant="outline" size="sm" disabled={disabled} onClick={onShuffle}>
          <Shuffle strokeWidth={1.75} />
          Shuffle
        </Button>
      )}
    </>
  );
}
