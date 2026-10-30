import { useMemo } from "react";

import { cn } from "@/shared/lib/utils";

import { logFrequencies, logPosition, response } from "./response";

const W = 1000;
const H = 120;
const POINTS = logFrequencies(160);
// SVG path commands.
const MOVE_TO = "M";
const LINE_TO = "L";

/**
 * The curve the filters draw (without the level change ahead of them), on
 * a log frequency axis that lines up with the faders below: each band's
 * centre falls in the middle of its fader's column.
 */
export function ResponseGraph({
  freqs,
  gains,
  range,
  dim,
}: {
  freqs: readonly number[];
  gains: readonly number[];
  /** dB at the top edge (and its negative at the bottom). */
  range: number;
  dim?: boolean;
}) {
  const path = useMemo(() => {
    const db = response(freqs, gains, 0, POINTS);
    const y = (d: number) => H / 2 - (Math.max(-range, Math.min(range, d)) / range) * (H / 2);
    return POINTS.map(
      (f, i) => `${i ? LINE_TO : MOVE_TO}${(logPosition(f) * W).toFixed(1)} ${y(db[i]).toFixed(2)}`,
    ).join("");
  }, [freqs, gains, range]);

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      preserveAspectRatio="none"
      aria-hidden
      className="block h-28 w-full overflow-visible"
    >
      <line
        x1={0}
        x2={W}
        y1={H / 2}
        y2={H / 2}
        className="stroke-line"
        strokeWidth={1}
        vectorEffect="non-scaling-stroke"
      />
      <path
        d={`${path}L${W} ${H / 2}L0 ${H / 2}Z`}
        className={cn(
          "fill-ink transition-opacity duration-150",
          dim ? "opacity-[0.03]" : "opacity-[0.07]",
        )}
      />
      <path
        d={path}
        fill="none"
        strokeWidth={2}
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
        className={cn("transition-colors duration-150", dim ? "stroke-ink-faint" : "stroke-ink")}
      />
    </svg>
  );
}
