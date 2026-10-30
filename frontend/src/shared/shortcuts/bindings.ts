import { SystemService } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";

import { keys } from "@/shared/lib/keys";

import type { Chord } from "./chord";
import { registeredShortcuts, resolveBindings, type Bindings, type Overrides } from "./registry";

const none: Overrides = {};

/**
 * The user's shortcut overrides, from the settings query: a settings save
 * (optimistic) or Go's settings:changed replaces it, so bindings apply live.
 */
export function useShortcutOverrides(): Overrides {
  const q = useQuery({
    queryKey: keys.settings,
    queryFn: () => SystemService.Settings(),
    select: (s) => s.shortcuts,
  });
  return q.data ?? none;
}

/** Every registered shortcut's current chords. */
export function useShortcutBindings(): Bindings {
  const overrides = useShortcutOverrides();
  return resolveBindings(registeredShortcuts(), overrides);
}

/** One shortcut's current chords (for a tooltip or a hint): [] when unbound. */
export function useBinding(id: string): readonly Chord[] {
  return useShortcutBindings().get(id) ?? [];
}
