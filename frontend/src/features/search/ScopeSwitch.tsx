import { cn } from "@/shared/lib/utils";

import type { SearchScope } from "./search-params";

const scopes: { value: SearchScope; label: string }[] = [
  { value: "catalog", label: "Apple Music" },
  { value: "library", label: "Library" },
];

/** Where the search looks: the Apple Music catalogue or the library on this computer. */
export function ScopeSwitch({
  scope,
  onChange,
}: {
  scope: SearchScope;
  onChange: (scope: SearchScope) => void;
}) {
  return (
    <div
      role="group"
      aria-label="Search in"
      className="flex h-8 items-center rounded-md bg-hover p-0.5"
    >
      {scopes.map((s) => {
        const on = s.value === scope;
        return (
          <button
            key={s.value}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(s.value)}
            className={cn(
              "h-7 rounded-[5px] px-3 text-sub font-medium transition-colors duration-100",
              on ? "bg-raised text-ink" : "text-ink-muted hover:text-ink",
            )}
          >
            {s.label}
          </button>
        );
      })}
    </div>
  );
}
