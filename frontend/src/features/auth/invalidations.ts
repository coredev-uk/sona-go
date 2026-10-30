import type { QueryKey } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

/** The fields of auth.Status that decide what cached data is still valid. */
export interface AuthShape {
  state: string;
  storefrontId?: string;
  backend?: string;
}

/**
 * Queries invalidated by an auth change. Signing in or out swaps the
 * library (Go wipes or syncs it) and the catalogue's personal fields; a new
 * storefront changes every catalogue answer.
 */
export function authInvalidations(prev: AuthShape | undefined, next: AuthShape): QueryKey[] {
  if (!prev) return [];
  const out: QueryKey[] = [];
  const wasIn = prev.state === "signed_in";
  const isIn = next.state === "signed_in";
  if (wasIn !== isIn) out.push(keys.library, keys.catalog);
  else if ((prev.storefrontId ?? "") !== (next.storefrontId ?? "")) out.push(keys.catalog);
  if (wasIn !== isIn || prev.backend !== next.backend) out.push(keys.diagnostics);
  return out;
}
