import { keys } from "@/shared/lib/keys";
import type { FeatureModule } from "@/shared/module";

import { UpdateNotice } from "./UpdateNotice";

/**
 * Update notice: Go checks GitHub after the first paint (unless turned off
 * in Settings → Diagnostics) and sends "updates:available" when a newer
 * release exists; the sidebar footer then links to it.
 */
export const updatesModule: FeatureModule<never> = {
  id: "updates",
  routes: [],
  eventHandlers: {
    "updates:available": (data, { qc }) => qc.setQueryData(keys.updates, data),
  },
  shellSlots: { SidebarFooter: UpdateNotice },
};
