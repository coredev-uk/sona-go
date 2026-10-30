import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

/** A compact select for a page's one filter (sort order, chart genre). */
export function Choice({
  label,
  value,
  options,
  onChange,
  width = "w-44",
}: {
  label: string;
  value: string;
  options: readonly { value: string; label: string }[];
  onChange: (value: string) => void;
  width?: string;
}) {
  // Radix Select cannot hold "" as a value; "" (Apple's default) maps to a token.
  const token = (v: string) => v || "_";
  return (
    <Select value={token(value)} onValueChange={(v) => onChange(v === "_" ? "" : v)}>
      <SelectTrigger aria-label={label} className={`h-8 text-ui ${width}`}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value} value={token(o.value)}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
