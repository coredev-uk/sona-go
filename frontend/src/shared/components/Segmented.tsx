import { cn } from "@/shared/lib/utils";

/**
 * A short row of exclusive choices shown together (two to four): one is
 * pressed. For longer lists use a Select. `label` names the group for
 * assistive technology.
 */
export function Segmented<V extends string>({
  value,
  options,
  label,
  onChange,
}: {
  value: V;
  options: readonly { value: V; label: string }[];
  label: string;
  onChange: (value: V) => void;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="flex h-8 items-center rounded-md bg-hover p-0.5"
    >
      {options.map((o) => {
        const on = o.value === value;
        return (
          <button
            key={o.value}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(o.value)}
            className={cn(
              "h-7 rounded-[5px] px-3 text-sub font-medium whitespace-nowrap transition-colors duration-100",
              on ? "bg-raised text-ink" : "text-ink-muted hover:text-ink",
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
