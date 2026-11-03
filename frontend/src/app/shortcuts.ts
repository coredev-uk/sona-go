import { t } from "@lingui/core/macro";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, PanelLeft } from "lucide-react";
import { useEffect } from "react";

import { announceUnavailable } from "@/shared/lib/unavailable";
import type { Shortcut, ShortcutScope } from "@/shared/module";
import { useShortcutBindings } from "@/shared/shortcuts/bindings";
import { chordKey, eventChord, isPlainChord, type Chord } from "@/shared/shortcuts/chord";
import { registeredShortcuts, type Bindings } from "@/shared/shortcuts/registry";

import { useLayout } from "./shell/layout";

/** The shell's own commands: history and the sidebar. */
export const shellShortcuts: Shortcut[] = [
  {
    id: "shell.back",
    label: () => t`Go back`,
    group: "navigation",
    keys: ["Alt+ArrowLeft"],
    scope: "global",
    icon: ChevronLeft,
    run: ({ history }) => history.back(),
  },
  {
    id: "shell.forward",
    label: () => t`Go forward`,
    group: "navigation",
    keys: ["Alt+ArrowRight"],
    scope: "global",
    icon: ChevronRight,
    run: ({ history }) => history.forward(),
  },
  {
    id: "shell.sidebar",
    label: () => t`Show or hide the sidebar`,
    group: "view",
    keys: ["Ctrl+B"],
    scope: "global",
    icon: PanelLeft,
    run: () => useLayout.getState().toggleSidebar(),
  },
];

/** Open widgets whose keys belong to them. */
const OWNED = "[role=menu], [role=dialog], [role=alertdialog], [role=listbox]";

const TEXT_FIELD =
  "textarea, select, [contenteditable]:not([contenteditable=false]), input:not([type=checkbox], [type=radio], [type=button], [type=submit], [type=reset], [type=range], [type=color], [type=file])";

/** Controls that use Space, Enter, arrows or paging keys themselves. */
const CONTROL =
  "button, a[href], summary, input, select, textarea, [contenteditable], [role=button], [role=link], [role=slider], [role=spinbutton], [role=menuitem], [role=option], [role=combobox], [role=tab], [role=switch], [role=checkbox], [role=radio]";

const CONTROL_KEYS: ReadonlySet<string> = new Set([
  "Space",
  "Enter",
  "ArrowUp",
  "ArrowDown",
  "ArrowLeft",
  "ArrowRight",
  "Home",
  "End",
  "PageUp",
  "PageDown",
] as const);

/** Where focus is when a key arrives. */
export interface KeyFocus {
  /** In a text field. */
  typing: boolean;
  /** On a control (button, slider, link…). */
  control: boolean;
}

/**
 * Whether a shortcut in `scope` may take `chord` with focus as it is: a
 * text field keeps every key except global chords with Ctrl, Alt or Meta,
 * and a control keeps the plain keys it operates with.
 */
export function keyAllowed(scope: ShortcutScope = "page", chord: Chord, focus: KeyFocus): boolean {
  if (focus.typing) return scope === "global" && !isPlainChord(chord);
  return !(focus.control && isPlainChord(chord) && CONTROL_KEYS.has(chordKey(chord)));
}

/** Shortcuts by chord, in registration order (the first allowed one wins). */
export function chordIndex(list: readonly Shortcut[], bindings: Bindings): Map<Chord, Shortcut[]> {
  const out = new Map<Chord, Shortcut[]>();
  for (const s of list) {
    for (const chord of bindings.get(s.id) ?? []) {
      const at = out.get(chord);
      if (at) at.push(s);
      else out.set(chord, [s]);
    }
  }
  return out;
}

/**
 * The single window keydown listener for every registered shortcut, with
 * the user's bindings (re-indexed when settings change). Keys already
 * handled, composing text, or inside an open menu, dialog or listbox are
 * left alone; otherwise the first shortcut bound to the chord that is
 * allowed with the current focus and whose `when` holds takes the key.
 * One that is `unavailable` takes it too, but only says why.
 */
export function useShortcuts() {
  const navigate = useNavigate();
  const { history } = useRouter();
  const bindings = useShortcutBindings();

  useEffect(() => {
    const byChord = chordIndex(registeredShortcuts(), bindings);
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.isComposing) return;
      const chord = eventChord(e);
      const candidates = chord ? byChord.get(chord) : undefined;
      if (!chord || !candidates) return;
      const target = e.target instanceof HTMLElement ? e.target : null;
      if (target?.closest(OWNED)) return;
      const focus = { typing: !!target?.closest(TEXT_FIELD), control: !!target?.closest(CONTROL) };
      const shortcut = candidates.find(
        (s) => keyAllowed(s.scope, chord, focus) && (s.when?.() ?? true),
      );
      if (!shortcut) return;
      e.preventDefault();
      if (e.repeat && !shortcut.repeat) return;
      const why = shortcut.unavailable?.();
      if (why) {
        if (!e.repeat) announceUnavailable(why);
        return;
      }
      shortcut.run({ navigate, history });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [bindings, navigate, history]);
}
