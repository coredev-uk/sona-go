import type { ReactNode } from "react";

import { cn } from "@/shared/lib/utils";

/** The id of the step's h1; the page focuses it when the step changes. */
export const STEP_TITLE_ID = "setup-step-title";

/**
 * A step's title (the page's one h1) and what the step is for. The welcome
 * sets its title at the collection size; every other step at the page size.
 */
export function StepHeader({
  title,
  large,
  children,
}: {
  title: string;
  large?: boolean;
  children?: ReactNode;
}) {
  return (
    <header className="mb-8">
      <h1
        id={STEP_TITLE_ID}
        tabIndex={-1}
        className={cn(
          "text-balance text-ink focus:outline-none",
          large ? "text-display" : "text-page",
        )}
      >
        {title}
      </h1>
      {children && (
        <p className="mt-2 max-w-[60ch] text-ui text-pretty text-ink-muted">{children}</p>
      )}
    </header>
  );
}

/**
 * A setting this platform can't do yet, where its row would be: its name
 * and why there is no control, so the step is honest about what is missing.
 */
export function UnavailableRow({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="py-4">
      <p className="text-ui font-medium text-ink">{title}</p>
      <p className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">{children}</p>
    </div>
  );
}
