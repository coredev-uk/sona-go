import { useLingui } from "@lingui/react/macro";
import { CircleSlash } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/shared/lib/utils";

import { usePlatformName } from "./platform";

/** A group's heading: an h3 under the section's h2. */
export function GroupHeading({ id, children }: { id: string; children: ReactNode }) {
  return (
    <h3 id={id} className="text-heading text-ink">
      {children}
    </h3>
  );
}

/**
 * Every select in a row: wide enough for most values, growing with a
 * longer one (a translation, an output device's name) instead of cutting it.
 */
export const SELECT_WIDTH = "h-8 min-w-44 max-w-72";

/**
 * A run of rows: set on the ground like a ledger, each row under one 1px
 * line, no box around them (docs/design.md, Settings).
 */
export function SettingsPanel({
  children,
  className,
}: {
  children?: ReactNode;
  className?: string;
}) {
  return <div className={cn("isolate flex flex-col", className)}>{children}</div>;
}

/** A section's groups (and any untitled block before them), stacked at one spacing. */
export function SettingsSections({ children }: { children: ReactNode }) {
  return <div className="flex flex-col gap-10">{children}</div>;
}

/** A section still loading: its rows' shape, as flat bars at the real text sizes. */
export function SettingsLoading({ rows = 4 }: { rows?: number }) {
  return (
    <div aria-busy className="flex flex-col">
      <div className="mb-3 h-4 w-28 rounded-sm bg-hover" />
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center justify-between gap-10 border-t border-line py-4">
          <div className="flex flex-col gap-2">
            <div className="h-3.5 w-40 rounded-sm bg-hover" />
            <div
              className="h-3 rounded-sm bg-hover"
              style={{ width: `${220 + ((i * 53) % 140)}px` }}
            />
          </div>
          <div className="h-8 w-44 rounded-md bg-hover" />
        </div>
      ))}
    </div>
  );
}

/**
 * A named group of rows within a section: a heading (an h3 under the
 * section's h2), optionally a line about the whole group, then its rows.
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
    <section aria-labelledby={id} className="flex flex-col gap-3">
      <div>
        <GroupHeading id={id}>{title}</GroupHeading>
        {description && (
          <div className="mt-1 max-w-[64ch] text-sub text-pretty text-ink-muted">{description}</div>
        )}
      </div>
      <SettingsPanel>{children}</SettingsPanel>
    </section>
  );
}

/**
 * A row's frame: the 1px line above it, and the wash Find a setting puts
 * on the row it lands on (`data-found`, see find.ts). `data-setting` is the
 * id Find a setting looks the row up by.
 */
export const ROW_FRAME =
  "relative border-t border-line py-4 before:pointer-events-none before:absolute before:inset-x-[-12px] before:inset-y-0 before:-z-10 before:rounded-md before:bg-hover before:opacity-0 before:transition-opacity before:duration-150 data-found:before:opacity-100";

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
      data-setting={id}
      className={cn(
        ROW_FRAME,
        stacked ? "flex flex-col gap-3" : "flex items-center justify-between gap-10",
      )}
    >
      <div className="min-w-0">
        <label htmlFor={id} id={`${id}-label`} className="text-ui font-medium text-ink">
          {title}
        </label>
        <p id={`${id}-desc`} className="mt-0.5 max-w-[60ch] text-sub text-pretty text-ink-muted">
          {description}
        </p>
      </div>
      <div className={cn(!stacked && "shrink-0")}>{children}</div>
    </div>
  );
}

/**
 * A setting this platform can't back (`Capabilities`): the row keeps its
 * place and title and says why there is no control, instead of vanishing.
 * Find a setting can land on it (it takes focus, but isn't a Tab stop).
 */
export function UnavailableRow({
  id,
  title,
  reason,
}: {
  id: string;
  title: string;
  /** Why; by default that the platform doesn't have it yet. */
  reason?: string;
}) {
  const { t } = useLingui();
  const platform = usePlatformName();
  return (
    <div
      data-setting={id}
      tabIndex={-1}
      aria-labelledby={`${id}-label`}
      aria-describedby={`${id}-desc`}
      role="group"
      className={cn(ROW_FRAME, "outline-none")}
    >
      <p id={`${id}-label`} className="text-ui font-medium text-ink-muted">
        {title}
      </p>
      <p
        id={`${id}-desc`}
        className="mt-0.5 flex max-w-[60ch] items-start gap-1.5 text-sub text-ink-muted"
      >
        <CircleSlash aria-hidden className="mt-0.5 size-3.5 shrink-0" strokeWidth={1.75} />
        <span>{reason ?? t`Not available on ${platform} yet.`}</span>
      </p>
    </div>
  );
}
