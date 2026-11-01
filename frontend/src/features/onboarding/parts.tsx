import type { ReactNode } from "react";

import { cn } from "@/shared/lib/utils";
import { Switch } from "@/shared/ui/switch";

/** The id of the step's h1; the page focuses it when the step changes. */
export const STEP_TITLE_ID = "setup-step-title";

/** A step's title (the page's one h1) and what the step is for. */
export function StepHeader({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <header className="mb-6">
      <h1
        id={STEP_TITLE_ID}
        tabIndex={-1}
        className="text-page text-balance text-ink focus:outline-none"
      >
        {title}
      </h1>
      {children && (
        <p className="mt-2 max-w-[60ch] text-ui text-pretty text-ink-muted">{children}</p>
      )}
    </header>
  );
}

/** The step's way on: its buttons, the main one first. */
export function StepActions({ children }: { children: ReactNode }) {
  return <div className="mt-8 flex flex-wrap items-center gap-3">{children}</div>;
}

/** Rows on the surface tone, divided by the structural line (as in Settings). */
export function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("divide-y divide-line rounded-lg bg-surface px-5", className)}>
      {children}
    </div>
  );
}

/** One switch with its label and what it does; the switch saves at once. */
export function SwitchRow({
  id,
  title,
  description,
  checked,
  disabled,
  onCheckedChange,
}: {
  id: string;
  title: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (on: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-8 py-4">
      <div className="min-w-0">
        <label htmlFor={id} className="text-ui font-medium text-ink">
          {title}
        </label>
        <p id={`${id}-desc`} className="mt-0.5 max-w-[56ch] text-sub text-ink-muted">
          {description}
        </p>
      </div>
      <Switch
        id={id}
        aria-describedby={`${id}-desc`}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}
