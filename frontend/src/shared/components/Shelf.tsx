import { Link, type LinkProps } from "@tanstack/react-router";
import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/shared/lib/utils";

interface ShelfProps {
  title: string;
  /** "See all": the section's full list as its own page. */
  more?: Pick<LinkProps, "to" | "params" | "search">;
  /** Extra controls at the right of the heading (e.g. "Show all" in place). */
  actions?: ReactNode;
  className?: string;
  children: ReactNode;
}

/** The heading-row control style: "See all", "Show all", "Show all lyrics". */
const control =
  "-mr-1 inline-flex h-7 items-center gap-0.5 rounded-md px-1.5 text-sub text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink";

/** A heading-row toggle that opens the shelf's content in place. */
export function ShelfToggle({
  open,
  onToggle,
  children,
}: {
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <button type="button" aria-expanded={open} onClick={onToggle} className={control}>
      {children}
    </button>
  );
}

/**
 * A titled section of a page: the heading at 16/600 with "See all" (or
 * other controls) at its right, then the content. Shelves stack 40px apart.
 */
export function Shelf({ title, more, actions, className, children }: ShelfProps) {
  return (
    <section aria-label={title} className={cn("mt-10 first:mt-0", className)}>
      <div className="mb-3 flex min-h-7 items-center justify-between gap-4">
        <h2 className="truncate text-heading text-ink">{title}</h2>
        <div className="flex shrink-0 items-center gap-1">
          {actions}
          {more && (
            <Link {...more} className={control}>
              See all
              <ChevronRight aria-hidden className="size-3.5" strokeWidth={1.75} />
            </Link>
          )}
        </div>
      </div>
      {children}
    </section>
  );
}
