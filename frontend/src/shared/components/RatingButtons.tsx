import { Star, ThumbsDown } from "lucide-react";

import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { useItemState, type StateItem, type StateKind } from "@/shared/lib/item-state";
import { rateItem } from "@/shared/lib/library-actions";
import { cn } from "@/shared/lib/utils";

/**
 * Favourite and Suggest Less for an item's header (album, playlist,
 * artist, song, music video), synced to Apple and shown from the batched
 * library state. `compact` draws the 28px icon size for bars and rows.
 */
export function RatingButtons({
  kind,
  item,
  name,
  compact,
  suggestLess = true,
}: {
  kind: StateKind;
  item: StateItem;
  name: string;
  compact?: boolean;
  /** Offer Suggest Less too (default); the now-playing bar shows Favourite only. */
  suggestLess?: boolean;
}) {
  const state = useItemState(kind, item);
  const rating = state.data?.rating ?? 0;
  const favourite = rating === 1;
  const less = rating === -1;
  const size = compact ? "icon-xs" : "icon-sm";
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size={size}
            aria-label={favourite ? `Undo favourite for ${name}` : `Favourite ${name}`}
            aria-pressed={favourite}
            disabled={state.isPending}
            onClick={() => rateItem(kind, item, name, favourite ? "unfavorite" : "favorite")}
            className={cn("text-ink-muted hover:text-ink", favourite && "text-ink")}
          >
            <Star fill={favourite ? "currentColor" : "none"} strokeWidth={1.75} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{favourite ? "Undo favourite" : "Favourite"}</TooltipContent>
      </Tooltip>
      {suggestLess && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size={size}
              aria-label={less ? `Undo suggest less for ${name}` : `Suggest less like ${name}`}
              aria-pressed={less}
              disabled={state.isPending}
              onClick={() =>
                rateItem(kind, item, name, less ? "undo-suggest-less" : "suggest-less")
              }
              className={cn("text-ink-muted hover:text-ink", less && "bg-active text-ink")}
            >
              <ThumbsDown fill={less ? "currentColor" : "none"} strokeWidth={1.75} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{less ? "Undo suggest less" : "Suggest less"}</TooltipContent>
        </Tooltip>
      )}
    </>
  );
}
