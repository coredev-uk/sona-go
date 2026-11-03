import type { Item } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { Dialog as DialogPrimitive } from "radix-ui";
import { Search } from "lucide-react";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";

import { itemMeta, libraryCards, useLibrarySearch } from "@/features/search";
import { Artwork } from "@/shared/components/Artwork";
import { itemLink, itemShape } from "@/shared/items/links";
import { playItem } from "@/shared/items/play";
import { cn } from "@/shared/lib/utils";
import { shortcutGroups, type Shortcut } from "@/shared/module";
import { useShortcutBindings } from "@/shared/shortcuts/bindings";
import { groupTitle } from "@/shared/shortcuts/groups";
import { eventChord } from "@/shared/shortcuts/chord";
import { BindingKeys, ChordKeys } from "@/shared/shortcuts/Keys";
import { registeredShortcuts } from "@/shared/shortcuts/registry";
import { DialogOverlay, DialogPortal, DialogTitle } from "@/shared/ui/dialog";

import { rankCommands } from "./rank";
import { useCommands } from "./store";

/** Library lookups follow the typing after this pause. */
const LIBRARY_DELAY = 150;
const LIBRARY_PER_KIND = 4;
const SEARCH_KEY = "search";
const KEY_UP = "ArrowUp";
const KEY_DOWN = "ArrowDown";
const KEY_RUN = "Enter";
const KEY_CLOSE = "Escape";
/** The keys shown in the footer hints. */
const HINT_KEYS = {
  up: KEY_UP,
  down: KEY_DOWN,
  run: KEY_RUN,
  close: KEY_CLOSE,
} as const;
const MAX_LIBRARY = 8;

type Entry =
  | { type: "command"; key: string; shortcut: Shortcut }
  | { type: "item"; key: string; item: Item }
  | { type: "search"; key: string; term: string };

interface Section {
  title: string;
  entries: Entry[];
}

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setV(value), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return v;
}

const command = (shortcut: Shortcut): Entry => ({
  type: "command",
  key: `c:${shortcut.id}`,
  shortcut,
});

/**
 * The palette's sections: with nothing typed, every available command by
 * group; with a query, the best matching commands, then library items from
 * the local index, then a search of Apple Music for the text.
 */
function sections(query: string, commands: readonly Shortcut[], items: readonly Item[]): Section[] {
  const q = query.trim();
  if (!q) {
    return shortcutGroups
      .map((g) => ({
        title: groupTitle(g),
        entries: commands.filter((s) => s.group === g).map(command),
      }))
      .filter((s) => s.entries.length);
  }
  const out: Section[] = [];
  const ranked = rankCommands(q, commands);
  if (ranked.length) out.push({ title: t`Commands`, entries: ranked.map(command) });
  if (items.length) {
    out.push({
      title: t`In your library`,
      entries: items.map((item) => ({ type: "item", key: `i:${item.kind}:${item.id}`, item })),
    });
  }
  out.push({ title: t`Apple Music`, entries: [{ type: "search", key: SEARCH_KEY, term: q }] });
  return out;
}

function Option({
  id,
  active,
  onPick,
  onHover,
  children,
}: {
  id: string;
  active: boolean;
  onPick: () => void;
  onHover: () => void;
  children: ReactNode;
}) {
  return (
    <div
      id={id}
      role="option"
      aria-selected={active}
      // Focus stays in the field; the pick happens on click.
      onMouseDown={(e) => e.preventDefault()}
      onClick={onPick}
      onMouseMove={onHover}
      className={cn(
        "flex min-h-9 cursor-default items-center gap-3 rounded-md px-2.5 py-1 text-ui text-ink",
        // The field keeps focus, so the active option carries the ring.
        active && "bg-active outline-2 -outline-offset-2 outline-ring",
      )}
    >
      {children}
    </div>
  );
}

/**
 * Ctrl+K: one field that finds pages, actions (every registered shortcut,
 * with its keys) and library items, and runs the chosen one. An ARIA
 * combobox over a grouped listbox; ↑/↓ move, Enter runs, Esc closes, and
 * focus returns where it was.
 */
export function CommandPalette() {
  const open = useCommands((s) => s.open === "palette");
  return (
    <DialogPrimitive.Root
      open={open}
      onOpenChange={(o) => !o && useCommands.getState().show("none")}
    >
      {open && <PaletteBody />}
    </DialogPrimitive.Root>
  );
}

function PaletteBody() {
  const { t } = useLingui();
  const navigate = useNavigate();
  const { history } = useRouter();
  const bindings = useShortcutBindings();
  const listId = useId();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const pending = useRef<(() => void) | null>(null);

  const term = useDebounced(query.trim(), LIBRARY_DELAY);
  const library = useLibrarySearch(term, LIBRARY_PER_KIND, term.length >= 2);
  const cards =
    query.trim() && term.length >= 2 && library.data ? libraryCards(library.data) : null;
  const items = cards
    ? [...cards.artists, ...cards.albums, ...cards.playlists, ...cards.songs].slice(0, MAX_LIBRARY)
    : [];

  const commands = registeredShortcuts().filter(
    (s) => s.palette !== false && (s.when?.() ?? true) && !s.unavailable?.(),
  );
  const list = sections(query, commands, items);
  const entries = list.flatMap((s) => s.entries);
  // Each section's first option index, for the options' ids.
  const starts = list.map((_, i) => list.slice(0, i).reduce((n, s) => n + s.entries.length, 0));
  const activeIndex = Math.min(active, entries.length - 1);
  const optionId = (i: number) => `${listId}-${i}`;

  useEffect(() => {
    if (activeIndex >= 0) {
      document.getElementById(`${listId}-${activeIndex}`)?.scrollIntoView({ block: "nearest" });
    }
  }, [activeIndex, listId]);

  const close = () => useCommands.getState().show("none");

  /** Closes, then runs `fn` once focus is back where it was (it may move it). */
  const runAfterClose = (fn: () => void) => {
    pending.current = fn;
    close();
  };

  const pick = (entry: Entry) => {
    switch (entry.type) {
      case "command":
        return runAfterClose(() => entry.shortcut.run({ navigate, history }));
      case "item": {
        const link = entry.item.kind === "song" ? undefined : itemLink(entry.item);
        return runAfterClose(() => (link ? void navigate(link) : playItem(entry.item)));
      }
      case "search":
        return runAfterClose(() => void navigate({ to: "/search", search: { q: entry.term } }));
    }
  };

  const onKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    const count = entries.length;
    switch (e.key) {
      case "ArrowDown":
      case "ArrowUp":
        e.preventDefault();
        if (count) setActive((activeIndex + (e.key === "ArrowDown" ? 1 : -1) + count) % count);
        return;
      case "Enter":
        e.preventDefault();
        if (entries[activeIndex]) pick(entries[activeIndex]);
        return;
    }
    // The palette's own key closes it again.
    const chord = eventChord(e.nativeEvent);
    if (chord && bindings.get("commands.palette")?.includes(chord)) {
      e.preventDefault();
      close();
    }
  };

  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        aria-describedby={undefined}
        onCloseAutoFocus={(e) => {
          e.preventDefault();
          // Another command surface took over: it owns focus now.
          if (useCommands.getState().open !== "none") return;
          useCommands.getState().restoreFocus();
          const fn = pending.current;
          pending.current = null;
          fn?.();
        }}
        className="fixed top-[14vh] left-1/2 z-50 flex w-[min(600px,calc(100vw-2rem))] -translate-x-1/2 flex-col overflow-hidden rounded-lg border border-line bg-raised shadow-float duration-150 outline-none data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:slide-in-from-bottom-1"
      >
        <DialogTitle className="sr-only">
          <Trans>Command palette</Trans>
        </DialogTitle>
        <div className="flex h-12 shrink-0 items-center gap-3 border-b border-line px-4">
          <Search aria-hidden className="size-4 shrink-0 text-ink-faint" strokeWidth={1.75} />
          <input
            type="text"
            role="combobox"
            aria-label={t`Find a page, action or music in your library`}
            aria-autocomplete="list"
            aria-expanded
            aria-controls={listId}
            aria-activedescendant={activeIndex >= 0 ? optionId(activeIndex) : undefined}
            autoComplete="off"
            spellCheck={false}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setActive(0);
            }}
            onKeyDown={onKeyDown}
            placeholder={t`Go to a page, run an action, find music in your library`}
            className="h-full min-w-0 flex-1 bg-transparent text-ui text-ink outline-none placeholder:text-ink-faint focus-visible:outline-none"
          />
        </div>
        <div
          id={listId}
          role="listbox"
          aria-label={t`Results`}
          className="max-h-[min(56vh,440px)] overflow-y-auto p-1.5"
        >
          {list.map((section, si) => {
            const headingId = `${listId}-${section.title}`;
            return (
              <div key={section.title} role="group" aria-labelledby={headingId}>
                <div
                  id={headingId}
                  role="presentation"
                  className="px-2.5 pt-2.5 pb-1 text-meta text-ink-faint"
                >
                  {section.title}
                </div>
                {section.entries.map((entry, ei) => {
                  const i = starts[si] + ei;
                  const common = {
                    id: optionId(i),
                    active: i === activeIndex,
                    onPick: () => pick(entry),
                    onHover: () => setActive(i),
                  };
                  if (entry.type === "command") {
                    const s = entry.shortcut;
                    const Icon = s.icon;
                    return (
                      <Option key={entry.key} {...common}>
                        <span className="grid size-4 shrink-0 place-items-center text-ink-muted">
                          {Icon && <Icon aria-hidden className="size-4" strokeWidth={1.75} />}
                        </span>
                        <span className="min-w-0 flex-1 truncate">{s.label()}</span>
                        <BindingKeys chords={(bindings.get(s.id) ?? []).slice(0, 2)} />
                      </Option>
                    );
                  }
                  if (entry.type === "item") {
                    const it = entry.item;
                    return (
                      <Option key={entry.key} {...common}>
                        <Artwork src={it.art.small} size={32} alt="" shape={itemShape(it)} />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate">{it.name}</span>
                          <span className="block truncate text-sub text-ink-muted">
                            {itemMeta(it)}
                          </span>
                        </span>
                      </Option>
                    );
                  }
                  const term = entry.term;
                  return (
                    <Option key={entry.key} {...common}>
                      <Search
                        aria-hidden
                        className="size-4 shrink-0 text-ink-muted"
                        strokeWidth={1.75}
                      />
                      <span className="min-w-0 flex-1 truncate">
                        <Trans>Search Apple Music for “{term}”</Trans>
                      </span>
                    </Option>
                  );
                })}
              </div>
            );
          })}
        </div>
        <div
          aria-hidden
          className="flex h-9 shrink-0 items-center gap-4 border-t border-line px-4 text-meta text-ink-faint"
        >
          <span className="flex items-center gap-1.5">
            <ChordKeys chord={HINT_KEYS.up} />
            <ChordKeys chord={HINT_KEYS.down} />
            <Trans>Move</Trans>
          </span>
          <span className="flex items-center gap-1.5">
            <ChordKeys chord={HINT_KEYS.run} />
            <Trans>Run</Trans>
          </span>
          <span className="flex items-center gap-1.5">
            <ChordKeys chord={HINT_KEYS.close} />
            <Trans>Close</Trans>
          </span>
        </div>
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}
