import { useLingui } from "@lingui/react/macro";
import { ListEnd, ListStart, Play, X } from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";

/** Where a queue row's menu opens; `keyboard` starts focus on its first item. */
export interface QueueMenuAt {
  x: number;
  y: number;
  keyboard: boolean;
}

/**
 * A queue row's menu at a point: right-click, Shift+F10 or the Menu key, or
 * the row's "more" button (below the button). Play, Play next and Move to
 * end (each only where it changes something: `canNext` and `canEnd`), then
 * Remove. It mounts open and unmounts through `onClose`; focus returns to
 * the row, not to the invisible anchor.
 */
export function QueueRowMenu({
  at,
  canNext,
  canEnd,
  onPlay,
  onNext,
  onEnd,
  onRemove,
  onClose,
  returnFocus,
}: {
  at: QueueMenuAt;
  canNext: boolean;
  canEnd: boolean;
  onPlay: () => void;
  onNext: () => void;
  onEnd: () => void;
  onRemove: () => void;
  onClose: () => void;
  returnFocus: () => HTMLElement | null | undefined;
}) {
  const { t } = useLingui();
  return (
    <DropdownMenu open onOpenChange={(o) => !o && onClose()}>
      <DropdownMenuTrigger asChild>
        <span
          aria-hidden
          className="pointer-events-none fixed size-0"
          style={{ left: at.x, top: at.y }}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        sideOffset={2}
        className="w-48"
        // Radix focuses the content itself when it mounts open; a keyboard
        // open starts on the first item instead, after that.
        ref={
          at.keyboard
            ? (el: HTMLDivElement | null) => {
                if (!el) return;
                requestAnimationFrame(() =>
                  el.querySelector<HTMLElement>("[role^=menuitem]:not([data-disabled])")?.focus(),
                );
              }
            : undefined
        }
        onCloseAutoFocus={(e) => {
          const el = returnFocus();
          if (!el) return;
          e.preventDefault();
          el.focus({ preventScroll: true });
        }}
      >
        <DropdownMenuItem onSelect={onPlay}>
          <Play />
          {t`Play`}
        </DropdownMenuItem>
        {canNext && (
          <DropdownMenuItem onSelect={onNext}>
            <ListStart />
            {t`Play next`}
          </DropdownMenuItem>
        )}
        {canEnd && (
          <DropdownMenuItem onSelect={onEnd}>
            <ListEnd />
            {t({
              message: "Move to end",
              comment:
                "Queue row menu: moves the song to the end of the songs the user queued, before AutoPlay's. Apple has no such item.",
            })}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={onRemove}>
          <X />
          {t`Remove`}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
