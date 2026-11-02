import { Link, type LinkProps } from "@tanstack/react-router";
import type { ReactNode } from "react";

import { Button } from "@/shared/ui/button";
import { cn } from "@/shared/lib/utils";

/** A router link, or a button for an action that isn't a page (opening the account dialog). */
export type EmptyStateAction =
  ({ label: string } & Pick<LinkProps, "to" | "search">) | { label: string; onClick: () => void };

interface EmptyStateProps {
  title: string;
  children: ReactNode;
  /** At most one action. */
  action?: EmptyStateAction;
  /**
   * "box": a dashed box where the block's data will appear (lists, panels).
   * "covers": one row of empty cover slots above the text, for pages whose
   * content is a grid of artwork (albums, playlists, shelves).
   */
  frame?: "box" | "covers";
  className?: string;
}

/**
 * The frame for a block with nothing in it yet: where the data will appear,
 * one sentence-case title, one sentence, at most one action.
 */
export function EmptyState({ title, children, action, frame = "box", className }: EmptyStateProps) {
  const text = (
    <>
      <h2 className="text-heading text-ink">{title}</h2>
      <p className="max-w-[60ch] text-ui text-ink-muted">{children}</p>
      {action &&
        ("onClick" in action ? (
          <Button size="sm" variant="outline" className="mt-4" onClick={action.onClick}>
            {action.label}
          </Button>
        ) : (
          <Button asChild size="sm" variant="outline" className="mt-4">
            <Link to={action.to} search={action.search}>
              {action.label}
            </Link>
          </Button>
        ))}
    </>
  );

  if (frame === "covers") {
    return (
      <div className={cn("flex flex-col items-start gap-1", className)}>
        {/* One row of slots at the grid's real tile size; extra slots wrap out of view. */}
        <div
          aria-hidden
          className="mb-5 grid w-full auto-rows-[0] grid-cols-[repeat(auto-fill,minmax(160px,1fr))] grid-rows-[auto] gap-x-6 overflow-hidden"
        >
          {/* Enough slots to fill a 4K-wide row at the 160px minimum. */}
          {Array.from({ length: 20 }, (_, i) => (
            <div
              key={i}
              className="aspect-square rounded-sm border border-dashed border-control-line"
            />
          ))}
        </div>
        {text}
      </div>
    );
  }

  return (
    <div
      className={cn(
        "flex flex-col items-start gap-1 rounded-lg border border-dashed border-control-line px-6 py-7",
        className,
      )}
    >
      {text}
    </div>
  );
}
