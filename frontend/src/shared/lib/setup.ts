import { create } from "zustand";

/**
 * Whether the first-run setup is open over the app (features/onboarding
 * draws it as a window-wide layer). Shared so Settings → General's "Set up
 * Sona again" can open it without importing the onboarding feature.
 */
export const useSetupOpen = create<{ open: boolean }>(() => ({ open: false }));

/** Opens the setup at its first step. */
export function openSetup() {
  useSetupOpen.setState({ open: true });
}

export function closeSetup() {
  useSetupOpen.setState({ open: false });
}
