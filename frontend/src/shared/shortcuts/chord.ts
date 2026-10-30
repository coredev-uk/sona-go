/**
 * Key chords as strings: modifiers in a fixed order, then one key —
 * "Ctrl+K", "Shift+ArrowRight", "Alt+1", "?", "Space". Letters are upper
 * case; "+" is "Plus". Shift is part of the chord for letters and named
 * keys, but not for other characters: "?" is whatever produces "?" on the
 * user's layout, so it is never "Shift+?".
 *
 * A binding is one or more chords separated by spaces, or "none".
 */

import { t } from "@lingui/core/macro";

/* eslint-disable lingui/no-unlocalized-strings -- KeyboardEvent key identifiers and chord syntax, never shown as text; chordParts/chordText translate them for display. */
export type Chord = string;

/** The binding that means "no key". */
export const NO_KEYS = "none";

const modifierOrder = ["Ctrl", "Alt", "Shift", "Meta"] as const;
type Modifier = (typeof modifierOrder)[number];

const modifierAliases: Record<string, Modifier> = {
  ctrl: "Ctrl",
  control: "Ctrl",
  alt: "Alt",
  option: "Alt",
  shift: "Shift",
  meta: "Meta",
  super: "Meta",
  cmd: "Meta",
  win: "Meta",
};

const namedKeys = [
  "Space",
  "Enter",
  "Escape",
  "Tab",
  "Backspace",
  "Delete",
  "Insert",
  "Home",
  "End",
  "PageUp",
  "PageDown",
  "ArrowUp",
  "ArrowDown",
  "ArrowLeft",
  "ArrowRight",
  "Plus",
  ...Array.from({ length: 24 }, (_, i) => `F${i + 1}`),
];

/** Lower-case spellings people write, to the canonical key name. */
const keyAliases: Record<string, string> = {
  ...Object.fromEntries(namedKeys.map((k) => [k.toLowerCase(), k])),
  esc: "Escape",
  return: "Enter",
  del: "Delete",
  ins: "Insert",
  pgup: "PageUp",
  pgdn: "PageDown",
  up: "ArrowUp",
  down: "ArrowDown",
  left: "ArrowLeft",
  right: "ArrowRight",
  spacebar: "Space",
  " ": "Space",
  "+": "Plus",
  comma: ",",
  period: ".",
  slash: "/",
};

/** Keys whose press alone never forms a chord. */
const lone = new Set([
  "Control",
  "Shift",
  "Alt",
  "Meta",
  "AltGraph",
  "CapsLock",
  "NumLock",
  "ScrollLock",
  "Fn",
  "FnLock",
  "Hyper",
  "Super",
  "OS",
  "Dead",
  "Compose",
  "Process",
  "Unidentified",
]);

const isLetter = (c: string) => /^\p{L}$/u.test(c);

/** The canonical key name for `raw`, or null if it is not a key a chord can use. */
function canonicalKey(raw: string): string | null {
  if (raw.length === 1 || [...raw].length === 1) {
    if (raw === " ") return "Space";
    if (raw === "+") return "Plus";
    if (isLetter(raw)) {
      const upper = raw.toUpperCase();
      return [...upper].length === 1 ? upper : raw;
    }
    return /^\p{C}$/u.test(raw) ? null : raw;
  }
  return keyAliases[raw.toLowerCase()] ?? null;
}

/** A single non-letter character: Shift is how the layout reaches it, not part of the chord. */
const shiftIsLayout = (key: string) => [...key].length === 1 && !isLetter(key);

function compose(mods: ReadonlySet<Modifier>, key: string): Chord {
  const parts: string[] = modifierOrder.filter(
    (m) => mods.has(m) && !(m === "Shift" && shiftIsLayout(key)),
  );
  parts.push(key);
  return parts.join("+");
}

/**
 * Parses a written chord ("ctrl+k", "Shift+Right", "Ctrl++") into its
 * canonical form, or null if it is not one chord.
 */
export function parseChord(text: string): Chord | null {
  const t = text.trim();
  if (!t) return null;
  let keyText: string;
  let modText: string;
  if (t === "+") {
    keyText = "+";
    modText = "";
  } else if (t.endsWith("++")) {
    keyText = "+";
    modText = t.slice(0, -2);
  } else {
    const at = t.lastIndexOf("+");
    keyText = t.slice(at + 1);
    modText = at < 0 ? "" : t.slice(0, at);
  }
  const key = canonicalKey(keyText.trim());
  if (!key) return null;
  const mods = new Set<Modifier>();
  if (modText) {
    for (const part of modText.split("+")) {
      const m = modifierAliases[part.trim().toLowerCase()];
      if (!m || mods.has(m)) return null;
      mods.add(m);
    }
  }
  return compose(mods, key);
}

/**
 * Parses a binding: space-separated chords, or "none" for no keys. Null if
 * any chord is invalid (the caller keeps the default then).
 */
export function parseBinding(text: string): Chord[] | null {
  const t = text.trim();
  if (t.toLowerCase() === NO_KEYS) return [];
  if (!t) return null;
  const chords: Chord[] = [];
  for (const part of t.split(/\s+/)) {
    const c = parseChord(part);
    if (!c) return null;
    if (!chords.includes(c)) chords.push(c);
  }
  return chords;
}

/** The binding string for `chords` ("none" when empty). */
export function formatBinding(chords: readonly Chord[]): string {
  return chords.length ? chords.join(" ") : NO_KEYS;
}

type KeyEventLike = Pick<
  KeyboardEvent,
  "key" | "code" | "ctrlKey" | "altKey" | "shiftKey" | "metaKey"
> & { getModifierState?: (key: string) => boolean };

/** The chord a keydown event makes, or null for a lone modifier or an unknown key. */
export function eventChord(e: KeyEventLike): Chord | null {
  if (lone.has(e.key)) return null;
  let key = canonicalKey(e.key);
  if (!key) return null;
  let { ctrlKey: ctrl, altKey: alt } = e;
  const single = [...key].length === 1;
  // AltGr reports Ctrl+Alt on Linux; the character it typed is the key.
  if (single && e.getModifierState?.("AltGraph")) ctrl = alt = false;
  // Ctrl/Alt with a non-Latin layout: use the physical Latin letter, as
  // other desktop apps do, so Ctrl+K works on a Cyrillic layout.
  if (single && isLetter(key) && !/^[A-Z]$/.test(key) && (ctrl || alt || e.metaKey)) {
    const m = /^Key([A-Z])$/.exec(e.code);
    if (m) key = m[1];
  }
  const mods = new Set<Modifier>();
  if (ctrl) mods.add("Ctrl");
  if (alt) mods.add("Alt");
  if (e.shiftKey) mods.add("Shift");
  if (e.metaKey) mods.add("Meta");
  return compose(mods, key);
}

/** True when the chord has no Ctrl, Alt or Meta: a key that types or moves the caret. */
export function isPlainChord(chord: Chord): boolean {
  return !/(^|\+)(Ctrl|Alt|Meta)\+/.test(chord);
}

/** The key of a chord, without its modifiers. */
export function chordKey(chord: Chord): string {
  if (chord === "Plus" || chord.endsWith("+Plus")) return "Plus";
  return chord.slice(chord.lastIndexOf("+") + 1);
}

/* eslint-enable lingui/no-unlocalized-strings */

/** Key and modifier names, shown and spoken alike. */
const keyNames: Record<string, () => string> = {
  Ctrl: () => t({ message: "Ctrl", context: "keyboard key" }),
  Alt: () => t({ message: "Alt", context: "keyboard key" }),
  Shift: () => t({ message: "Shift", context: "keyboard key" }),
  Meta: () => t({ message: "Super", context: "keyboard key" }),
  Space: () => t({ message: "Space", context: "keyboard key" }),
  Enter: () => t({ message: "Enter", context: "keyboard key" }),
  Escape: () => t({ message: "Escape", context: "keyboard key" }),
  Tab: () => t({ message: "Tab", context: "keyboard key" }),
  Backspace: () => t({ message: "Backspace", context: "keyboard key" }),
  Delete: () => t({ message: "Delete", context: "keyboard key" }),
  Insert: () => t({ message: "Insert", context: "keyboard key" }),
  Home: () => t({ message: "Home", context: "keyboard key" }),
  End: () => t({ message: "End", context: "keyboard key" }),
  PageUp: () => t({ message: "Page Up", context: "keyboard key" }),
  PageDown: () => t({ message: "Page Down", context: "keyboard key" }),
  Plus: () => t({ message: "Plus", context: "keyboard key" }),
};

/** Keycap glyphs where they differ from the name. */
const shown: Record<string, () => string> = {
  ArrowUp: () => "↑",
  ArrowDown: () => "↓",
  ArrowLeft: () => "←",
  ArrowRight: () => "→",
  Escape: () => t({ message: "Esc", context: "keyboard key" }),
  Plus: () => "+",
};

/** Words for assistive tech where they differ from the name. */
const spoken: Record<string, () => string> = {
  ArrowUp: () => t({ message: "Up Arrow", context: "keyboard key" }),
  ArrowDown: () => t({ message: "Down Arrow", context: "keyboard key" }),
  ArrowLeft: () => t({ message: "Left Arrow", context: "keyboard key" }),
  ArrowRight: () => t({ message: "Right Arrow", context: "keyboard key" }),
  ",": () => t({ message: "Comma", context: "keyboard key" }),
  ".": () => t({ message: "Period", context: "keyboard key" }),
  "/": () => t({ message: "Slash", context: "keyboard key" }),
  "?": () => t({ message: "Question mark", context: "keyboard key" }),
};

function split(chord: Chord): string[] {
  const key = chordKey(chord);
  const mods = chord
    .slice(0, chord.length - key.length)
    .split("+")
    .filter(Boolean);
  return [...mods, key];
}

/** The keycaps of a chord, as shown: ["Ctrl", "←"]. */
export function chordParts(chord: Chord): string[] {
  return split(chord).map((p) => (shown[p] ?? keyNames[p])?.() ?? p);
}

/** The chord in words for assistive tech: "Ctrl+Left Arrow". */
export function chordText(chord: Chord): string {
  return split(chord)
    .map((p) => (spoken[p] ?? keyNames[p])?.() ?? p)
    .join("+");
}
