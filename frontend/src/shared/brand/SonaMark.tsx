import { cn } from "@/shared/lib/utils";

/** The Sona mark: a record label seen from above, amber with a spindle hole. */
export function SonaMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" aria-hidden="true" className={cn("size-5 shrink-0", className)}>
      <circle cx="16" cy="16" r="15" fill="var(--amber)" />
      <circle cx="16" cy="16" r="3" fill="var(--surface)" />
    </svg>
  );
}
