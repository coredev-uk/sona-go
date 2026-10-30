import type { ReactNode } from "react";

interface PageHeaderProps {
  title: string;
  /** One line about the data or the object; never about the interface. */
  subline?: ReactNode;
  actions?: ReactNode;
}

export function PageHeader({ title, subline, actions }: PageHeaderProps) {
  return (
    <header className="mb-6 flex items-end justify-between gap-6">
      <div className="min-w-0">
        <h1 className="truncate text-page text-ink">{title}</h1>
        {subline && <p className="mt-1 text-sub text-ink-muted">{subline}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}
