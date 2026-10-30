import { Trans, useLingui } from "@lingui/react/macro";
import { Play, Shuffle } from "lucide-react";

import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

type PlayButtonsProps = {
  disabled?: boolean;
  onPlay: () => void;
  /** Omitted for a single item (a song): no Shuffle button. */
  onShuffle?: () => void;
} & (
  | {
      /** What plays, by name; the button reads "Play {label}": "OK Computer". */
      label: string;
      playLabel?: never;
    }
  | {
      /** The Play button's whole accessible name when what plays is not a name: "Play your songs". */
      playLabel: string;
      label?: never;
    }
);

/**
 * The collection's play control (the amber disc: amber means playing) and
 * a quiet Shuffle beside it.
 */
export function PlayButtons({ label, playLabel, disabled, onPlay, onShuffle }: PlayButtonsProps) {
  const { t } = useLingui();
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="icon"
            aria-label={playLabel ?? t`Play ${label}`}
            disabled={disabled}
            onClick={onPlay}
            className="size-10 rounded-full bg-play text-on-play hover:bg-play/90 disabled:bg-hover disabled:text-ink-faint disabled:opacity-100"
          >
            <Play fill="currentColor" strokeWidth={0} className="size-[18px] translate-x-px" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <Trans>Play</Trans>
        </TooltipContent>
      </Tooltip>
      {onShuffle && (
        <Button variant="outline" size="sm" disabled={disabled} onClick={onShuffle}>
          <Shuffle strokeWidth={1.75} />
          <Trans>Shuffle</Trans>
        </Button>
      )}
    </>
  );
}
