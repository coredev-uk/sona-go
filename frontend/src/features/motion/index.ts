import { MotionService } from "@bindings/services";

import type { FeatureModule } from "@/shared/module";

import { MotionProvider } from "./MotionProvider";
import { usePower } from "./policy";

/**
 * Motion artwork: whether Apple's looping covers may play (the setting,
 * reduce effects, the system's reduced motion, Go's power source, and the
 * codecs WebKit decodes), given to covers through the shell Provider slot
 * (useMotionState).
 */
export const motionModule: FeatureModule<never> = {
  id: "motion",
  routes: [],
  eventHandlers: {
    "power:changed": (data) => usePower.setState({ power: data }),
  },
  init: () => {
    void MotionService.Power().then(
      (power) => usePower.setState({ power }),
      () => {},
    );
  },
  shellSlots: { Provider: MotionProvider },
};
