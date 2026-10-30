import * as React from "react";
import { cn } from "@/shared/lib/utils";
import { Slider as SliderPrimitive } from "radix-ui";

/**
 * Radix renders the role="slider" element as the Thumb, not the Root, so the
 * accessible name, description, value text and id (for `aria-labelledby`
 * from a row label) go on the thumb; on the Root they would label nothing.
 */
function Slider({
  className,
  defaultValue,
  value,
  min = 0,
  max = 100,
  id,
  "aria-label": ariaLabel,
  "aria-labelledby": ariaLabelledBy,
  "aria-describedby": ariaDescribedBy,
  "aria-valuetext": ariaValueText,
  ...props
}: React.ComponentProps<typeof SliderPrimitive.Root>) {
  const _values = React.useMemo(
    () => (Array.isArray(value) ? value : Array.isArray(defaultValue) ? defaultValue : [min, max]),
    [value, defaultValue, min, max],
  );
  const single = _values.length === 1;

  return (
    <SliderPrimitive.Root
      data-slot="slider"
      defaultValue={defaultValue}
      value={value}
      min={min}
      max={max}
      className={cn(
        "group/slider relative flex h-4 w-full touch-none items-center select-none data-[disabled]:opacity-50 data-[orientation=vertical]:h-full data-[orientation=vertical]:min-h-44 data-[orientation=vertical]:w-auto data-[orientation=vertical]:flex-col",
        className,
      )}
      {...props}
    >
      <SliderPrimitive.Track
        data-slot="slider-track"
        className={cn(
          "relative grow overflow-hidden rounded-full bg-ink-faint/40 data-[orientation=horizontal]:h-1 data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-1",
        )}
      >
        <SliderPrimitive.Range
          data-slot="slider-range"
          className={cn(
            "absolute bg-(--slider-range,var(--primary)) data-[orientation=horizontal]:h-full data-[orientation=vertical]:w-full",
          )}
        />
      </SliderPrimitive.Track>
      {Array.from({ length: _values.length }, (_, index) => (
        <SliderPrimitive.Thumb
          data-slot="slider-thumb"
          key={index}
          id={single ? id : undefined}
          aria-label={single ? ariaLabel : undefined}
          aria-labelledby={single ? ariaLabelledBy : undefined}
          aria-describedby={single ? ariaDescribedBy : undefined}
          aria-valuetext={single ? ariaValueText : undefined}
          // Always shown: the thumb, not the fill colour, is what marks the value.
          className="block size-3 shrink-0 rounded-full bg-ink focus-visible:ring-4 focus-visible:ring-ring focus-visible:outline-hidden disabled:pointer-events-none"
        />
      ))}
    </SliderPrimitive.Root>
  );
}

export { Slider };
