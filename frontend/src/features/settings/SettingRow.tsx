import type { ReactNode } from "react";

import { cn } from "@/shared/lib/utils";

/** A group's heading: an h3 under the section's h2. */
export function GroupHeading({ id, children }: { id: string; children: ReactNode }) {
  return (
    <h3 id={id} className="text-ui font-semibold text-ink">
      {children}
    </h3>
  );
}

/**
 * Every select in a row: wide enough for most values, growing with a
 * longer one (a translation, an output device's name) instead of cutting it.
 */
export const SELECT_WIDTH = "h-8 min-w-44 max-w-72";

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

/** A section's groups (and any panel before them), stacked at one spacing. */
export function SettingsSections({ children }: { children: ReactNode }) {
  return <div className="flex flex-col gap-8">{children}</div>;
}

/**
 * A named group of rows within a section: a heading (an h3 under the
 * section's h2) over one panel, optionally with a line about the whole group.
 */
export function SettingsGroup({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section aria-labelledby={id} className="flex flex-col gap-2">
      <div>
        <GroupHeading id={id}>{title}</GroupHeading>
        {description && (
          <div className="mt-0.5 max-w-[64ch] text-sub text-ink-muted">{description}</div>
        )}
      </div>
      <SettingsPanel>{children}</SettingsPanel>
    </section>
  );
}

/**
 * One setting: its label and what it does on the left, the control on the
 * right. The label is `${id}-label`, for controls `<label for>` cannot name
 * (sliders take `aria-labelledby`).
 */
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
        <label htmlFor={id} id={`${id}-label`} className="text-ui font-medium text-ink">
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
