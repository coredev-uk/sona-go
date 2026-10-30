import { useQueryClient } from "@tanstack/react-query";
import { useEffect, type ReactNode } from "react";

import { useAuth } from "@/features/auth";
import { ConfirmHost } from "@/shared/components/ConfirmDialog";
import { useContentPolicy } from "@/shared/lib/content-policy";
import { bindItemStates } from "@/shared/lib/item-state";

/**
 * Shell provider: gives the item-state helpers the query client (so menu
 * actions update every view of an item), keeps the explicit-content policy
 * in step with the account (restricted accounts see explicit items
 * disabled everywhere) and hosts the confirmation dialog removals use.
 */
export function LibraryProvider({ children }: { children: ReactNode }) {
  bindItemStates(useQueryClient());
  const restricted = useAuth().data?.restricted ?? false;
  useEffect(() => useContentPolicy.getState().setRestricted(restricted), [restricted]);
  return (
    <>
      {children}
      <ConfirmHost />
    </>
  );
}
