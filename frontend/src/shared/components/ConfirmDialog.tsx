import { Trans } from "@lingui/react/macro";

import { useConfirm } from "@/shared/lib/confirm";
import { Button } from "@/shared/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog";

/** Renders the pending `confirm()` question (shared/lib/confirm); mount once in the shell. */
export function ConfirmHost() {
  const open = useConfirm((s) => s.open);
  const settle = useConfirm((s) => s.settle);
  return (
    <Dialog open={!!open} onOpenChange={(o) => !o && settle(false)}>
      <DialogContent role="alertdialog" showCloseButton={false} className="max-w-[420px] gap-5 p-5">
        {open && (
          <>
            <DialogHeader className="gap-1.5">
              <DialogTitle className="text-heading text-ink">{open.title}</DialogTitle>
              <DialogDescription className="text-ui text-ink-muted">{open.body}</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" size="sm" onClick={() => settle(false)}>
                <Trans>Cancel</Trans>
              </Button>
              <Button
                variant={open.destructive ? "destructive" : "default"}
                size="sm"
                autoFocus
                onClick={() => settle(true)}
              >
                {open.confirmLabel}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
