import type { Item } from "@bindings/services";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight } from "lucide-react";

import { openExternal } from "./cover";
import { itemLink } from "./links";

/** Text links to editorial pages, in balanced columns; web-only links say so. */
export function LinkList({ title, items }: { title: string; items: readonly Item[] }) {
  return (
    // An untitled list continues the list above it (Apple splits long ones).
    <section aria-label={title || "More links"} className={title ? "mt-10 first:mt-0" : "mt-2"}>
      {title && <h2 className="mb-3 text-heading text-ink">{title}</h2>}
      <ul className="columns-[200px] gap-x-6">
        {items.map((item, i) => {
          const link = itemLink(item);
          const cls =
            "inline-flex h-8 max-w-full items-center gap-1 truncate rounded-md text-ui text-ink-muted underline-offset-2 transition-colors duration-100 hover:text-ink hover:underline";
          return (
            <li key={`${i}:${item.name}`} className="break-inside-avoid">
              {link ? (
                <Link {...link} className={cls}>
                  {item.name}
                </Link>
              ) : (
                <button type="button" className={cls} onClick={() => openExternal(item)}>
                  {item.name}
                  <ArrowUpRight aria-label="Opens in your browser" className="size-3.5" />
                </button>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
