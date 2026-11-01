import { Trans, useLingui } from "@lingui/react/macro";
import { RotateCcw, Search } from "lucide-react";
import { useId, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";

import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { confirm } from "@/shared/lib/confirm";
import { cn } from "@/shared/lib/utils";
import { shortcutGroups, type Shortcut } from "@/shared/module";
import { chordParts, chordText, eventChord, type Chord } from "@/shared/shortcuts/chord";
import { groupTitle } from "@/shared/shortcuts/groups";
import { BindingKeys, ChordKeys } from "@/shared/shortcuts/Keys";
import {
  conflictingIds,
  conflictsFor,
  defaultChords,
  rebind,
  registeredShortcuts,
  resolveBindings,
  type Bindings,
  type Overrides,
} from "@/shared/shortcuts/registry";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";

import { useSettings, useUpdateSettings } from "./queries";
import { SettingsPanel } from "./SettingRow";

/** Chords the recorder refuses: they move focus. */
// eslint-disable-next-line lingui/no-unlocalized-strings -- chord names, not shown
const RESERVED = new Set(["Tab", "Shift+Tab"]);

/** A chord waiting for the user to settle a conflict. */
interface Pending {
  chord: Chord;
  with: Shortcut[];
}

function ShortcutRow({
  shortcut,
  bindings,
  conflicted,
  overridden,
  onSave,
}: {
  shortcut: Shortcut;
  bindings: Bindings;
  conflicted: boolean;
  overridden: boolean;
  onSave: (chords: Chord[], steal?: boolean) => void;
}) {
  const { t, i18n } = useLingui();
  const [recording, setRecording] = useState(false);
  const [pending, setPending] = useState<Pending | null>(null);
  const [refused, setRefused] = useState("");
  const noteId = useId();
  // A key the recorder took: its keyup must not click the button again.
  const captured = useRef(false);
  const recorder = useRef<HTMLButtonElement>(null);
  const chords = bindings.get(shortcut.id) ?? [];
  const all = registeredShortcuts();
  const others = conflicted
    ? chords.flatMap((c) => conflictsFor(all, bindings, shortcut.id, c))
    : [];

  const label = shortcut.label();
  const orList = (cs: readonly Chord[]) =>
    new Intl.ListFormat(i18n.locale, { type: "disjunction" }).format(cs.map(chordText));
  const defaults = orList(defaultChords(shortcut));
  const otherNames = others.map((o) => `“${o.label()}”`).join(", ");
  const pendingNames = pending?.with.map((o) => `“${o.label()}”`).join(", ");

  const stop = () => {
    setRecording(false);
    setRefused("");
  };
  // The choice's buttons go with it, so focus returns to the recorder.
  const settle = (steal: boolean) => {
    if (steal && pending) onSave([pending.chord], true);
    setPending(null);
    recorder.current?.focus();
  };

  const onKeyDown = (e: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (!recording) {
      captured.current = false;
      return;
    }
    // Every key goes to the recorder, not the app's shortcuts.
    e.preventDefault();
    e.stopPropagation();
    captured.current = true;
    const chord = eventChord(e.nativeEvent);
    if (!chord) return;
    if (chord === "Escape") return stop();
    if (chord === "Backspace" || chord === "Delete") {
      stop();
      onSave([]);
      return;
    }
    if (RESERVED.has(chord)) {
      const key = chordText(chord);
      setRefused(t`${key} moves between controls. Choose another key.`);
      return;
    }
    stop();
    const clash = conflictsFor(all, bindings, shortcut.id, chord);
    if (clash.length) setPending({ chord, with: clash });
    else onSave([chord]);
  };

  return (
    <div className="py-1.5">
      <div className="flex min-h-8 items-center justify-between gap-6">
        <span id={`${noteId}-label`} className="min-w-0 text-ui text-ink">
          {label}
        </span>
        <div className="flex shrink-0 items-center gap-1.5">
          <button
            ref={recorder}
            type="button"
            aria-labelledby={`${noteId}-label ${noteId}-keys`}
            aria-describedby={recording || conflicted ? noteId : undefined}
            aria-pressed={recording}
            onClick={() => {
              if (captured.current) {
                captured.current = false;
                return;
              }
              setPending(null);
              setRecording((r) => !r);
              setRefused("");
            }}
            onKeyUp={(e) => captured.current && e.preventDefault()}
            onPointerDown={() => (captured.current = false)}
            onBlur={stop}
            onKeyDown={onKeyDown}
            className={cn(
              "flex h-8 min-w-36 items-center justify-end gap-1.5 rounded-md border px-2 transition-colors duration-100",
              recording
                ? "border-ink bg-raised"
                : "border-control-line/40 hover:border-control-line hover:bg-raised",
              conflicted && !recording && "border-danger/50",
            )}
          >
            {recording ? (
              // Named by what it shows while recording (Label in Name).
              <span id={`${noteId}-keys`} className="text-sub text-ink-muted">
                <Trans>Press keys…</Trans>
              </span>
            ) : (
              // The keycaps as shown ("Ctrl B", "Esc"), so the name contains
              // the visible label (2.5.3) for speech input.
              <span id={`${noteId}-keys`} className="sr-only">
                {chords.length
                  ? new Intl.ListFormat(i18n.locale, { type: "disjunction" }).format(
                      chords.map((c) => chordParts(c).join(" ")),
                    )
                  : t`not set`}
              </span>
            )}
            {recording ? null : chords.length ? (
              <BindingKeys chords={chords} />
            ) : (
              <span aria-hidden className="text-sub text-ink-faint">
                <Trans>Not set</Trans>
              </span>
            )}
          </button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={defaults ? t`Reset ${label} to ${defaults}` : t`Reset ${label} to no key`}
            title={t`Reset to default`}
            disabled={!overridden}
            onClick={() => {
              onSave(defaultChords(shortcut));
              // This button hides once the keys are back to the default.
              recorder.current?.focus();
            }}
            className="text-ink-muted hover:text-ink disabled:invisible"
          >
            <RotateCcw strokeWidth={1.75} />
          </Button>
        </div>
      </div>
      <div id={noteId} aria-live="polite" className="text-sub">
        {recording && (
          // Keyed so a refusal is a new paragraph, which WebKitGTK announces.
          <p key={refused} className="mt-1.5 text-ink-muted">
            {refused || t`Press the new keys. Esc cancels, Backspace removes the shortcut.`}
          </p>
        )}
        {!recording && conflicted && others.length > 0 && (
          <p className="mt-1.5 text-danger">
            <Trans>Also used by {otherNames}. Only the first one runs.</Trans>
          </p>
        )}
      </div>
      {pending && (
        <div
          role="alert"
          onKeyDown={(e) => {
            if (e.key === "Escape") settle(false);
          }}
          className="mt-2 flex items-center justify-between gap-4 rounded-md bg-hover px-3 py-2"
        >
          <p className="min-w-0 text-sub text-ink">
            <Trans>
              <ChordKeys chord={pending.chord} className="mr-1 align-middle" /> is used by{" "}
              {pendingNames}.
            </Trans>
          </p>
          <div className="flex shrink-0 gap-2">
            <Button variant="outline" size="sm" onClick={() => settle(false)}>
              <Trans>Cancel</Trans>
            </Button>
            <Button size="sm" autoFocus onClick={() => settle(true)}>
              <Trans>Use it here</Trans>
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

/**
 * Settings → Shortcuts: every registered shortcut by group with its keys.
 * Click a binding and press the new keys; a key another shortcut uses in
 * an overlapping scope asks first and moves it here. Changes save to Go's
 * settings (`shortcuts`) and apply at once.
 */
export function ShortcutsSection() {
  const { t } = useLingui();
  const settings = useSettings();
  const update = useUpdateSettings();
  const [filter, setFilter] = useState("");

  if (settings.error) {
    return (
      <ErrorBlock
        title={t`Couldn't read Sona's settings`}
        error={settings.error}
        onRetry={() => void settings.refetch()}
      />
    );
  }
  const s = settings.data;
  if (!s) return <SettingsPanel className="h-72" />;

  const overrides: Overrides = s.shortcuts ?? {};
  const all = registeredShortcuts();
  const bindings = resolveBindings(all, overrides);
  const conflicted = conflictingIds(all, bindings);
  const save = (next: Overrides) => update.mutate({ ...s, shortcuts: next });
  const nothingToReset = Object.keys(overrides).length === 0;

  const query = filter.trim();
  const f = query.toLowerCase();
  const shown = f
    ? all.filter(
        (x) =>
          x.label().toLowerCase().includes(f) ||
          (bindings.get(x.id) ?? []).some((c) => chordText(c).toLowerCase().includes(f)),
      )
    : all;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-3">
        <div className="relative min-w-0 flex-1">
          <Search
            aria-hidden
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-faint"
            strokeWidth={1.75}
          />
          <Input
            type="search"
            aria-label={t`Filter shortcuts`}
            placeholder={t`Filter by action or key`}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="h-8 pl-9"
          />
        </div>
        <Button
          variant="outline"
          size="sm"
          // aria-disabled, not disabled: after a reset it keeps focus
          // instead of dropping it to <body>.
          aria-disabled={nothingToReset || undefined}
          onClick={() => {
            if (nothingToReset) return;
            void confirm({
              title: t`Reset every shortcut?`,
              body: t`All shortcuts go back to their default keys.`,
              confirmLabel: t`Reset all`,
            }).then((ok) => ok && save({}));
          }}
          className="aria-disabled:pointer-events-none aria-disabled:opacity-50"
        >
          <Trans>Reset all</Trans>
        </Button>
      </div>
      {shortcutGroups.map((group) => {
        const list = shown.filter((x) => x.group === group);
        if (!list.length) return null;
        return (
          <section key={group} aria-labelledby={`shortcuts-${group}`}>
            <h3 id={`shortcuts-${group}`} className="mb-2 text-ui font-semibold text-ink">
              {groupTitle(group)}
            </h3>
            <SettingsPanel>
              {list.map((x) => (
                <ShortcutRow
                  key={x.id}
                  shortcut={x}
                  bindings={bindings}
                  conflicted={conflicted.has(x.id)}
                  overridden={x.id in overrides}
                  onSave={(chords, steal) => save(rebind(all, overrides, x.id, chords, steal))}
                />
              ))}
            </SettingsPanel>
          </section>
        );
      })}
      <div role="status">
        {shown.length === 0 && (
          <p className="py-6 text-center text-sub text-ink-muted">
            <Trans>No shortcut matches “{query}”.</Trans>
          </p>
        )}
      </div>
    </div>
  );
}
