import type { ReactNode } from "react";

import { cn } from "@/shared/lib/utils";

/** A settings panel: rows on the surface tone, divided by the structural line. */
export function SettingsPanel({
  children,
  className,
}: {
  children?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("divide-y divide-line rounded-lg bg-surface px-5", className)}>
      {children}
    </div>
  );
}

/** One setting: its label and what it does on the left, the control on the right. */
export function SettingRow({
  id,
  title,
  description,
  children,
  stacked,
}: {
  id: string;
  title: string;
  description: ReactNode;
  children: ReactNode;
  /** Control under the text (wide inputs). */
  stacked?: boolean;
}) {
  return (
    <div
      className={cn(
        "py-4",
        stacked ? "flex flex-col gap-3" : "flex items-center justify-between gap-8",
      )}
    >
      <div className="min-w-0">
        <label htmlFor={id} className="text-ui font-medium text-ink">
          {title}
        </label>
        <p id={`${id}-desc`} className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">
          {description}
        </p>
      </div>
      <div className={cn(!stacked && "shrink-0")}>{children}</div>
    </div>
  );
}
