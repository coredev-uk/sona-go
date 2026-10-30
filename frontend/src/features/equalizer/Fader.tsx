import { useLingui } from "@lingui/react/macro";
import { Slider as SliderPrimitive } from "radix-ui";

import { cn } from "@/shared/lib/utils";

import { formatDb } from "./curve";

/**
 * One vertical fader: its value above, the cap on a slim track filled from
 * the 0 dB line (or from the top for a cut-only control), its name below.
 */
export function Fader({
  label,
  name,
  value,
  min,
  max,
  dim,
  onChange,
  onCommit,
}: {
  /** Accessible name: "947 Hz", "Preamp". */
  label: string;
  /** Shown under the fader: "947", "1.9k", "Pre". */
  name: string;
  value: number;
  min: number;
  max: number;
  /** The equaliser is off: still editable, drawn quieter. */
  dim?: boolean;
  onChange: (v: number) => void;
  onCommit: (v: number) => void;
}) {
  const { t } = useLingui();
  const span = max - min;
  const db = formatDb(value);
  const zero = Math.min(Math.max(0, min), max);
  const at = (v: number) => ((v - min) / span) * 100; // % from the bottom
  const from = Math.min(at(zero), at(value));
  const to = Math.max(at(zero), at(value));
  return (
    <div className="flex min-w-0 flex-col items-center gap-2">
      <span
        aria-hidden
        className={cn("text-meta num", value === 0 ? "text-ink-faint" : "text-ink")}
      >
        {formatDb(value)}
      </span>
      <SliderPrimitive.Root
        orientation="vertical"
        min={min}
        max={max}
        step={0.5}
        value={[value]}
        onValueChange={([v]) => onChange(v)}
        onValueCommit={([v]) => onCommit(v)}
        className={cn(
          "group/fader relative flex h-40 w-8 touch-none flex-col items-center select-none",
          dim && "opacity-55",
        )}
      >
        <SliderPrimitive.Track className="relative h-full w-1 grow rounded-full bg-ink-faint/30">
          <span
            aria-hidden
            className="absolute inset-x-0 rounded-full bg-ink"
            style={{ bottom: `${from}%`, top: `${100 - to}%` }}
          />
        </SliderPrimitive.Track>
        <SliderPrimitive.Thumb
          aria-label={label}
          aria-valuetext={t`${db} dB`}
          className="block h-2.5 w-5 rounded-[3px] bg-ink shadow-[0_1px_3px_rgb(0_0_0/0.25)] transition-transform duration-100 group-hover/fader:scale-110 focus-visible:ring-4 focus-visible:ring-ring focus-visible:outline-hidden"
        />
      </SliderPrimitive.Root>
      <span aria-hidden className="text-meta text-ink-muted num">
        {name}
      </span>
    </div>
  );
}
