import { useLayoutEffect, useRef, useState } from "react";

import { cn } from "@/shared/lib/utils";

interface NotesProps {
  /** Plain text from Go: paragraphs separated by a blank line, line breaks kept. */
  text: string;
  /** Lines shown before "Read more" (default 4). */
  lines?: number;
  className?: string;
}

/**
 * Editorial notes and biographies at a reading measure: clamped to a few
 * lines with a "Read more" toggle when the text runs longer.
 */
export function Notes({ text, lines = 4, className }: NotesProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [overflows, setOverflows] = useState(false);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || open) return;
    const check = () => setOverflows(el.scrollHeight > el.clientHeight + 1);
    const ro = new ResizeObserver(check);
    ro.observe(el);
    check();
    return () => ro.disconnect();
  }, [open, text]);

  const paragraphs = text.split(/\n{2,}/);
  return (
    <div className={cn("max-w-[68ch]", className)}>
      <div
        ref={ref}
        className="text-ui whitespace-pre-line text-ink"
        style={
          open
            ? undefined
            : {
                display: "-webkit-box",
                WebkitBoxOrient: "vertical",
                WebkitLineClamp: lines,
                overflow: "hidden",
              }
        }
      >
        {paragraphs.map((p, i) => (
          <p key={i} className={i > 0 ? "mt-3" : undefined}>
            {p}
          </p>
        ))}
      </div>
      {(overflows || open) && (
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className="mt-1.5 rounded-sm text-sub font-medium text-ink-muted underline-offset-2 hover:text-ink hover:underline"
        >
          {open ? "Show less" : "Read more"}
        </button>
      )}
    </div>
  );
}
