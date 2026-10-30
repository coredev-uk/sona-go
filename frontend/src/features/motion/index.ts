import { MotionService } from "@bindings/services";

import type { FeatureModule } from "@/shared/module";

import { MotionProvider } from "./MotionProvider";
import { usePower } from "./policy";

/**
 * Motion artwork: whether Apple's looping covers may play (the setting,
 * reduce effects, the system's reduced motion, and Go's battery state),
 * given to covers through the shell Provider slot (useMotionAllowed).
 */
export const motionModule: FeatureModule<never> = {
  id: "motion",
  routes: [],
  eventHandlers: {
    "power:changed": (data) => usePower.setState({ onBattery: data.onBattery }),
  },
  init: () => {
    void MotionService.Power().then(
      (p) => usePower.setState({ onBattery: p.onBattery }),
      () => {},
    );
  },
  shellSlots: { Provider: MotionProvider },
};
