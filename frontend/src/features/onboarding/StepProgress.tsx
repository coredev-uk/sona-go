import { useLingui } from "@lingui/react/macro";
import { useRef, type KeyboardEvent } from "react";

import { cn } from "@/shared/lib/utils";

import { goToStep, setupSteps, useStepNames, type SetupStep } from "./steps";

/**
 * Where the setup is, as a strip of one segment per step, filled up to the
 * current one, each named under it; every segment is a button to its step (back, or ahead: each
 * step can be skipped). One Tab stop: the current step's segment, with ←/→
 * (and Home/End) moving between them, Enter or Space going there.
 */
export function StepProgress({ step }: { step: SetupStep }) {
  const { t } = useLingui();
  const names = useStepNames();
  const list = useRef<HTMLOListElement>(null);
  const at = setupSteps.indexOf(step);

  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const last = setupSteps.length - 1;
    const rtl = getComputedStyle(e.currentTarget).direction === "rtl";
    const moves: Record<string, number> = {
      ArrowRight: rtl ? i - 1 : i + 1,
      ArrowLeft: rtl ? i + 1 : i - 1,
      Home: 0,
      End: last,
    };
    const next = moves[e.key];
    if (next === undefined) return;
    e.preventDefault();
    const to = Math.min(Math.max(next, 0), last);
    list.current?.querySelectorAll("button")[to]?.focus();
  };

  return (
    <nav aria-label={t`Setup steps`}>
      <ol ref={list} className="flex gap-1">
        {setupSteps.map((s, i) => (
          <li key={s} className="flex-1">
            <button
              type="button"
              aria-current={s === step ? "step" : undefined}
              tabIndex={s === step ? 0 : -1}
              onClick={() => goToStep(s)}
              onKeyDown={(e) => onKeyDown(e, i)}
              className="group flex w-full cursor-pointer flex-col gap-1.5 rounded-sm pt-1 pb-0.5 text-start focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
            >
              <span
                aria-hidden
                className={cn(
                  "h-1 w-full rounded-full transition-colors duration-100",
                  i <= at ? "bg-ink" : "bg-ink/15 group-hover:bg-ink/35",
                )}
              />
              {/* The name under its segment; the current step's reads strongest. */}
              <span
                className={cn(
                  "truncate text-meta",
                  s === step ? "font-medium text-ink" : "text-ink-muted group-hover:text-ink",
                )}
              >
                {names[s]}
              </span>
            </button>
          </li>
        ))}
      </ol>
    </nav>
  );
}
