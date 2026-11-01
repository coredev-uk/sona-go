import { Trans } from "@lingui/react/macro";
import { Fragment } from "react";

import { cn } from "@/shared/lib/utils";

import { chordParts, chordText, type Chord } from "./chord";

/**
 * One chord as keycaps; assistive tech reads it as words ("Ctrl+Left Arrow").
 * A space separates the keycaps' text (the flex gap lays them out), so their
 * visible text reads "Ctrl B", not "CtrlB", where a label must contain it.
 */
export function ChordKeys({ chord, className }: { chord: Chord; className?: string }) {
  return (
    <kbd className={cn("inline-flex shrink-0 items-center gap-0.5 font-sans", className)}>
      <span className="sr-only">{chordText(chord)}</span>
      {chordParts(chord).map((part, i) => (
        <Fragment key={i}>
          {i > 0 && " "}
          <span
            aria-hidden
            className="inline-grid h-5 min-w-5 place-items-center rounded-sm border border-control-line px-1 text-meta font-medium text-ink-muted num"
          >
            {part}
          </span>
        </Fragment>
      ))}
    </kbd>
  );
}

/** A shortcut's chords side by side ("or" between them for assistive tech). */
export function BindingKeys({
  chords,
  className,
}: {
  chords: readonly Chord[];
  className?: string;
}) {
  return (
    <span className={cn("inline-flex flex-wrap items-center justify-end gap-1.5", className)}>
      {chords.map((c, i) => (
        <span key={c} className="inline-flex items-center gap-1.5">
          {i > 0 && (
            <span className="sr-only">
              <Trans>or</Trans>
            </span>
          )}
          <ChordKeys chord={c} />
        </span>
      ))}
    </span>
  );
}
