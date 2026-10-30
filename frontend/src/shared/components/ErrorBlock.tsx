import { Trans } from "@lingui/react/macro";
import { CircleAlert } from "lucide-react";

import { Button } from "@/shared/ui/button";
import { callFailure } from "@/shared/lib/errors";
import { cn } from "@/shared/lib/utils";

interface ErrorBlockProps {
  /** What failed, in the product's words: "Couldn't load this album." */
  title: string;
  error?: unknown;
  onRetry?: () => void;
  retrying?: boolean;
  className?: string;
}

/**
 * An error inside the block that failed: what failed, Go's reason, and a
 * Retry text button. `--danger` only on the small icon.
 */
export function ErrorBlock({ title, error, onRetry, retrying, className }: ErrorBlockProps) {
  const detail = callFailure(error).message;
  return (
    <div
      role="alert"
      className={cn(
        "flex items-start gap-3 rounded-lg border border-dashed border-control-line px-5 py-4",
        className,
      )}
    >
      <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0 text-danger" strokeWidth={1.75} />
      <div className="min-w-0 flex-1">
        <p className="text-ui font-medium text-ink">{title}</p>
        {detail && <p className="mt-0.5 text-sub break-words text-ink-muted">{detail}</p>}
      </div>
      {onRetry && (
        <Button
          variant="ghost"
          size="sm"
          onClick={onRetry}
          disabled={retrying}
          className="-my-1 shrink-0"
        >
          {retrying ? <Trans>Retrying…</Trans> : <Trans>Retry</Trans>}
        </Button>
      )}
    </div>
  );
}
