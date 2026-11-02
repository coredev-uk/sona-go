import { createContext } from "react";

/**
 * The setup's foot, beside its own buttons: a step puts its primary action
 * there (the Apple Music step's Sign in) so it stays in view while the step
 * scrolls. Null outside the setup or before the foot has mounted.
 */
export const SetupFoot = createContext<HTMLElement | null>(null);
