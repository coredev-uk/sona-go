import type { Item } from "@bindings/services";

import { ItemActionMenu } from "@/shared/actions/ActionMenu";

import { actionItemOf } from "./links";

/** The item's "more" menu, for kinds with registered actions; null otherwise. */
export function ItemMenu({ item }: { item: Item }) {
  const target = actionItemOf(item);
  return target ? <ItemActionMenu item={target} name={item.name} /> : null;
}
