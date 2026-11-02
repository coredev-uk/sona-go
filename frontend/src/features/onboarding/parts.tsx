import type { ReactNode } from "react";

import { cn } from "@/shared/lib/utils";
import { Switch } from "@/shared/ui/switch";

/** The id of the step's title (the dialog's title); the layer focuses it when the step changes. */
export const STEP_TITLE_ID = "setup-step-title";

/**
 * A part of a step: a heading one level under the step's title (h3; the
 * dialog's title is the h2), then its controls.
 */
export function StepGroup({
  id,
  title,
  children,
  className,
}: {
  id: string;
  title: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section aria-labelledby={id} className={cn("flex flex-col gap-2", className)}>
      <h3 id={id} className="text-ui font-semibold text-ink">
        {title}
      </h3>
      {children}
    </section>
  );
}

export interface Choice<T extends string> {
  value: T;
  label: string;
  hint: string;
  /** A picture over the label (the theme's windows). */
  picture?: ReactNode;
}

/**
 * One choice of two or three, as cards side by side: radio buttons, so Tab
 * reaches the chosen one and the arrow keys choose (each choice saves at
 * once). The chosen card takes an ink ring and a filled dot; the dot keeps
 * the choice readable without the ring's weight.
 */
export function ChoiceCards<T extends string>({
  name,
  labelledBy,
  value,
  choices,
  disabled,
  onChange,
}: {
  name: string;
  labelledBy: string;
  value: T | undefined;
  choices: readonly Choice<T>[];
  disabled?: boolean;
  onChange: (value: T) => void;
}) {
  return (
    <div
      role="radiogroup"
      aria-labelledby={labelledBy}
      className={cn("grid gap-2", choices.length > 2 ? "grid-cols-3" : "grid-cols-2")}
    >
      {choices.map((c) => {
        const checked = value === c.value;
        const id = `${name}-${c.value}`;
        return (
          <label key={c.value} className="group relative flex cursor-pointer">
            <input
              type="radio"
              name={name}
              value={c.value}
              checked={checked}
              disabled={disabled}
              aria-labelledby={`${id}-label`}
              aria-describedby={`${id}-hint`}
              onChange={() => onChange(c.value)}
              className="peer sr-only"
            />
            <span className="flex w-full flex-col gap-2.5 rounded-md bg-surface p-3 transition-colors duration-100 group-hover:bg-hover peer-checked:ring-2 peer-checked:ring-ink peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-ring peer-disabled:cursor-not-allowed peer-disabled:opacity-60">
              {c.picture}
              <span className="flex items-start gap-2.5">
                <span
                  aria-hidden
                  className={cn(
                    "mt-0.5 grid size-4 shrink-0 place-items-center rounded-full border",
                    checked ? "border-ink" : "border-control-line",
                  )}
                >
                  {checked && <span className="size-2 rounded-full bg-ink" />}
                </span>
                <span className="flex min-w-0 flex-col">
                  <span id={`${id}-label`} className="text-ui font-medium text-ink">
                    {c.label}
                  </span>
                  <span id={`${id}-hint`} className="text-sub text-ink-muted">
                    {c.hint}
                  </span>
                </span>
              </span>
            </span>
          </label>
        );
      })}
    </div>
  );
}

/**
 * An on/off choice: its name and one line on what it does, the switch at
 * the end. `children` sit under it while it applies (crossfade's length).
 */
export function SwitchRow({
  id,
  label,
  hint,
  checked,
  disabled,
  onChange,
  children,
}: {
  id: string;
  label: string;
  hint: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (on: boolean) => void;
  children?: ReactNode;
}) {
  return (
    <div className="py-3">
      <div className="flex items-center justify-between gap-6">
        <div className="min-w-0">
          <label htmlFor={id} id={`${id}-label`} className="text-ui font-medium text-ink">
            {label}
          </label>
          <p id={`${id}-hint`} className="text-sub text-pretty text-ink-muted">
            {hint}
          </p>
        </div>
        <Switch
          id={id}
          aria-describedby={`${id}-hint`}
          checked={checked}
          disabled={disabled}
          onCheckedChange={onChange}
        />
      </div>
      {children}
    </div>
  );
}

/** Switch rows (and their notes) as one list, divided by the structural line. */
export function SwitchList({ children }: { children: ReactNode }) {
  return <div className="flex flex-col divide-y divide-line">{children}</div>;
}

/** A flat block on the surface tone, for a step's one status (Widevine, the sign-in). */
export function Well({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("rounded-md bg-surface px-4 py-3", className)}>{children}</div>;
}

/** A step's loading state: a surface block at about the height of what will land. */
export function Loading({ className }: { className?: string }) {
  return <div aria-busy className={cn("rounded-md bg-surface", className)} />;
}
