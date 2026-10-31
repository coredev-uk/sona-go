import { PlatformService, type Capabilities } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

import { keys } from "./keys";

/** The last capabilities Go returned, for code outside React (a shortcut's `when`). */
let known: Capabilities | undefined;

/**
 * What this platform and build support (PlatformService.Capabilities):
 * fixed for the process, so fetched once and never refetched.
 */
export const capabilitiesQuery = {
  queryKey: keys.capabilities,
  queryFn: async () => (known = await PlatformService.Capabilities()),
  staleTime: Infinity,
  gcTime: Infinity,
};

/**
 * The platform's capabilities, or undefined until Go answers. A control
 * whose capability is false is hidden or explained, never shown broken;
 * undefined counts as false, so nothing appears only to stop working.
 * On Linux every capability that works today is true.
 */
export function useCapabilities(): Capabilities | undefined {
  return useQuery(capabilitiesQuery).data;
}

/** The capabilities once loaded (undefined before), for non-React code such as a shortcut's `when`. */
export function currentCapabilities(): Capabilities | undefined {
  return known;
}
