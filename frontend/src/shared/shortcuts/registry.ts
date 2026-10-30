import type { Shortcut, ShortcutScope } from "@/shared/module";

import { formatBinding, parseBinding, parseChord, type Chord } from "./chord";

/**
 * Every module's shortcuts, filled once by the app before the first render
 * (app/compose.ts), and the pure rules for bindings: user overrides over
 * defaults, conflicts, rebinding.
 */

let registered: readonly Shortcut[] = [];

/** Registers the app's shortcuts; throws on a duplicate id or an invalid default chord. */
export function registerShortcuts(list: readonly Shortcut[]): void {
  const ids = new Set(registered.map((s) => s.id));
  for (const s of list) {
    if (ids.has(s.id)) throw new Error(`shortcut ${s.id} is registered twice`);
    ids.add(s.id);
    for (const k of s.keys) {
      if (!parseChord(k)) throw new Error(`shortcut ${s.id}: invalid default key "${k}"`);
    }
  }
  registered = [...registered, ...list];
}

export function registeredShortcuts(): readonly Shortcut[] {
  return registered;
}

/** Settings' shortcut overrides: id → binding ("Ctrl+K", "none"). */
export type Overrides = Readonly<Partial<Record<string, string>>>;

/** The chords each shortcut answers to, by id. */
export type Bindings = ReadonlyMap<string, readonly Chord[]>;

/** A shortcut's default chords, canonical. */
export function defaultChords(s: Shortcut): Chord[] {
  return s.keys.flatMap((k) => parseChord(k) ?? []);
}

/**
 * Each shortcut's chords: its override when that parses ("none" = no keys),
 * otherwise its defaults. A blank or invalid override keeps the defaults;
 * overrides for ids not registered are ignored.
 */
export function resolveBindings(list: readonly Shortcut[], overrides: Overrides): Bindings {
  const out = new Map<string, readonly Chord[]>();
  for (const s of list) {
    const o = overrides[s.id];
    out.set(s.id, (o !== undefined && parseBinding(o)) || defaultChords(s));
  }
  return out;
}

/** Whether two scopes can be active at the same time. Distinct contexts cannot. */
export function scopesOverlap(a: ShortcutScope = "page", b: ShortcutScope = "page"): boolean {
  const base = (s: ShortcutScope) => s === "page" || s === "global";
  return a === b || base(a) || base(b);
}

/** Other shortcuts that answer to `chord` where `id` would. */
export function conflictsFor(
  list: readonly Shortcut[],
  bindings: Bindings,
  id: string,
  chord: Chord,
): Shortcut[] {
  const self = list.find((s) => s.id === id);
  return list.filter(
    (s) =>
      s.id !== id && bindings.get(s.id)?.includes(chord) && scopesOverlap(self?.scope, s.scope),
  );
}

/** The ids involved in any conflict: two shortcuts on one chord in overlapping scopes. */
export function conflictingIds(list: readonly Shortcut[], bindings: Bindings): Set<string> {
  const out = new Set<string>();
  for (const s of list) {
    for (const chord of bindings.get(s.id) ?? []) {
      if (conflictsFor(list, bindings, s.id, chord).length) out.add(s.id);
    }
  }
  return out;
}

/** `overrides` with `id` set to `chords`, dropping the entry when it equals the defaults. */
function withBinding(
  list: readonly Shortcut[],
  overrides: Overrides,
  id: string,
  chords: readonly Chord[],
): Partial<Record<string, string>> {
  const next: Partial<Record<string, string>> = { ...overrides };
  const s = list.find((x) => x.id === id);
  const defaults = s ? defaultChords(s) : [];
  const same = chords.length === defaults.length && chords.every((c, i) => c === defaults[i]);
  if (same) delete next[id];
  else next[id] = formatBinding(chords);
  return next;
}

/**
 * Binds `id` to `chords` (replacing its keys). With `steal`, shortcuts that
 * conflict with any of them lose that chord (and keep their others), so the
 * key does one thing again.
 */
export function rebind(
  list: readonly Shortcut[],
  overrides: Overrides,
  id: string,
  chords: readonly Chord[],
  steal = false,
): Partial<Record<string, string>> {
  let next = withBinding(list, overrides, id, chords);
  if (!steal) return next;
  for (const chord of chords) {
    for (const other of conflictsFor(list, resolveBindings(list, next), id, chord)) {
      const kept = (resolveBindings(list, next).get(other.id) ?? []).filter((c) => c !== chord);
      next = withBinding(list, next, other.id, kept);
    }
  }
  return next;
}
