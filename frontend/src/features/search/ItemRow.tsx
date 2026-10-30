import type { Item } from "@bindings/services";
import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Play } from "lucide-react";
import type { ReactNode } from "react";

import { Artwork } from "@/shared/components/Artwork";
import { ItemMenu } from "@/shared/items/ItemMenu";
import { itemLink, itemShape } from "@/shared/items/links";
import { canPlay, playItem } from "@/shared/items/play";
import { cn } from "@/shared/lib/utils";
import { ExplicitMark } from "@/shared/components/Badges";

import { itemMeta } from "./groups";

const ROW_CLASS =
  "group/row flex min-w-0 flex-1 items-center gap-3 rounded-md px-2 py-1.5 text-left focus-visible:outline-none";

interface ItemRowProps {
  item: Item;
  /** Called when the row is opened or played (recent searches). */
  onOpen?: (item: Item) => void;
  /** Rendered artwork edge in CSS px. */
  size?: number;
  actions?: boolean;
  className?: string;
}

function Body({ item, size }: { item: Item; size: number }) {
  return (
    <>
      <Artwork src={item.art.small} size={size} alt="" shape={itemShape(item)} />
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-1.5">
          <span className="truncate text-ui text-ink">{item.name}</span>
          {item.explicit && <ExplicitMark />}
        </span>
        <span className="block truncate text-sub text-ink-muted">{itemMeta(item)}</span>
      </span>
    </>
  );
}

/**
 * One result of any kind as a list row: artwork, name, "Kind · subtitle".
 * Songs play in place; every other kind opens its page.
 */
export function ItemRow({ item, onOpen, size = 40, actions = true, className }: ItemRowProps) {
  const { t } = useLingui();
  const link = item.kind === "song" ? undefined : itemLink(item);
  const cls = ROW_CLASS;
  let main: ReactNode;
  if (link) {
    main = (
      <Link {...link} data-result className={cls} onClick={() => onOpen?.(item)}>
        <Body item={item} size={size} />
      </Link>
    );
  } else if (canPlay(item)) {
    main = (
      // Named by its visible text (WCAG 2.5.3), with the action after it.
      <button
        type="button"
        data-result
        className={cls}
        onClick={() => {
          onOpen?.(item);
          void playItem(item);
        }}
      >
        <Body item={item} size={size} />
        <span className="sr-only">{t`Play`}</span>
        <Play
          aria-hidden
          className="size-3.5 shrink-0 text-ink-muted opacity-0 group-hover/row:opacity-100 group-focus-visible/row:opacity-100"
          fill="currentColor"
          strokeWidth={0}
        />
      </button>
    );
  } else {
    main = (
      <div className={cls}>
        <Body item={item} size={size} />
      </div>
    );
  }
  return (
    <div
      className={cn(
        "group/item flex items-center rounded-md pr-1 outline-offset-[-2px] transition-colors duration-100 focus-within:bg-hover hover:bg-hover has-[[data-result]:focus-visible]:outline-2 has-[[data-result]:focus-visible]:outline-ring has-[[data-result]:focus-visible]:outline-solid",
        className,
      )}
    >
      {main}
      {actions && (
        <div className="opacity-0 group-focus-within/item:opacity-100 group-hover/item:opacity-100 has-[[data-state=open]]:opacity-100">
          <ItemMenu item={item} />
        </div>
      )}
    </div>
  );
}
