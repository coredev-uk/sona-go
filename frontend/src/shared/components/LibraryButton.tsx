import { Source } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Check, Plus } from "lucide-react";

import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { useItemState, type StateItem } from "@/shared/lib/item-state";
import { addToLibrary, removeFromLibrary } from "@/shared/lib/library-actions";

/**
 * A header's library control: "Add" for a catalog item outside the
 * library (`addLabel` names it otherwise: "Pre-add" for an album not out
 * yet), "In library" once it is there (choosing it offers removal, with a
 * confirmation). Membership comes from the batched library state, so it
 * is right on every page without a sync.
 */
export function LibraryButton({
  kind,
  item,
  name,
  addLabel,
}: {
  kind: "songs" | "albums" | "playlists" | "music-videos";
  item: StateItem;
  name: string;
  addLabel?: string;
}) {
  const { t } = useLingui();
  const state = useItemState(kind, item);
  const inLibrary = state.data?.inLibrary ?? item.source === Source.SourceLibrary;
  if (!inLibrary) {
    return (
      <Button
        variant="outline"
        size="sm"
        disabled={state.isPending}
        onClick={() => void addToLibrary(kind, [item], name)}
      >
        <Plus strokeWidth={1.75} />
        {addLabel ?? <Trans>Add</Trans>}
      </Button>
    );
  }
  // Library playlists are deleted by the playlist editor, not here.
  if (kind === "playlists") {
    return (
      <span className="inline-flex h-8 items-center gap-1.5 px-2 text-ui text-ink-muted">
        <Check className="size-4" strokeWidth={1.75} aria-hidden />
        <Trans>In library</Trans>
      </span>
    );
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          aria-label={t`In library. Delete “${name}” from your library`}
          onClick={() => void removeFromLibrary(kind, item, name)}
        >
          <Check strokeWidth={1.75} />
          <Trans>In library</Trans>
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        <Trans>Delete from library</Trans>
      </TooltipContent>
    </Tooltip>
  );
}
