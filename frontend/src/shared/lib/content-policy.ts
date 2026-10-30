import { create } from "zustand";

interface ContentPolicy {
  /** The account restricts explicit content (Apple's itre flag). */
  restricted: boolean;
  setRestricted: (restricted: boolean) => void;
}

/**
 * The account's explicit-content policy. The library module keeps it in
 * step with the auth status; lists and cards read it to disable explicit
 * items on restricted accounts.
 */
export const useContentPolicy = create<ContentPolicy>((set) => ({
  restricted: false,
  setRestricted: (restricted) => set((s) => (s.restricted === restricted ? s : { restricted })),
}));

/** Whether an item is withheld from this account. */
export function useWithheld(explicit: boolean | undefined): boolean {
  const restricted = useContentPolicy((s) => s.restricted);
  return restricted && !!explicit;
}
