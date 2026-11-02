import { create } from "zustand";

/**
 * The account dialog (signing in, the account's details), which the auth
 * feature renders beside the top bar's account menu. Shared so every "Sign
 * in" action (empty states, the status banner, the player bar) can open it
 * without depending on the auth feature.
 */
export const useAccountDialog = create<{ open: boolean }>(() => ({ open: false }));

export function openAccountDialog() {
  useAccountDialog.setState({ open: true });
}

export function closeAccountDialog() {
  useAccountDialog.setState({ open: false });
}
