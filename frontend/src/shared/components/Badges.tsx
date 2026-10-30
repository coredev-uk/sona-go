import type { Traits } from "@bindings/services";

import { traitLabels } from "@/shared/lib/traits";
import { cn } from "@/shared/lib/utils";

/** The small "E" beside explicit titles in rows and cards. */
export function ExplicitMark({ className }: { className?: string }) {
  return (
    <span
      role="img"
      aria-label="Explicit"
      title="Explicit"
      className={cn(
        "grid size-3.5 shrink-0 place-items-center rounded-[3px] bg-ink-faint/25 text-[9px] leading-none font-semibold text-ink-muted",
        className,
      )}
    >
      E
    </span>
  );
}

function Tag({ children }: { children: string }) {
  return (
    <span className="inline-flex h-5 flex-none items-center rounded-[4px] border border-control-line px-1.5 text-[11px] leading-none font-medium whitespace-nowrap text-ink-muted">
      {children}
    </span>
  );
}

/**
 * The badge line of a collection or song header: Explicit, then Apple's
 * audio labels (Dolby Atmos, Lossless, Apple Digital Master). Renders
 * nothing when the item has none.
 */
export function Badges({
  explicit,
  traits,
  extra,
  className,
}: {
  explicit?: boolean;
  traits?: Traits;
  /** Further labels after Apple's audio ones (a video's "4K", "HDR"). */
  extra?: readonly string[];
  className?: string;
}) {
  const labels = [...traitLabels(traits), ...(extra ?? [])];
  if (!explicit && labels.length === 0) return null;
  return (
    <span className={cn("inline-flex flex-wrap items-center gap-1.5", className)}>
      {explicit && <Tag>Explicit</Tag>}
      {labels.map((l) => (
        <Tag key={l}>{l}</Tag>
      ))}
    </span>
  );
}
