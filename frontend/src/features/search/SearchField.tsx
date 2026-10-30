import type { Item, RecentSearch } from "@bindings/services";
import { useQuery } from "@tanstack/react-query";
import { useLocation, useNavigate, useRouterState } from "@tanstack/react-router";
import { Clock, Search, X } from "lucide-react";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";

import { SearchService } from "@bindings/services";
import { Artwork } from "@/shared/components/Artwork";
import { itemLink, itemShape } from "@/shared/items/links";
import { playItem } from "@/shared/items/play";
import { keys } from "@/shared/lib/keys";
import { cn } from "@/shared/lib/utils";
import { Input } from "@/shared/ui/input";

import { moveActive, splitMatch } from "./combobox";
import { itemMeta } from "./groups";
import { libraryCards, useRecentActions, useRecentSearches } from "./queries";
import { recentItem, recentLink } from "./recent";
import type { SearchScope } from "./search-params";

/** Suggestions follow the typing after this pause. */
const HINT_DELAY = 150;
const MAX_RECENT = 6;

type Option =
  | { type: "term"; term: string; display: string }
  | { type: "item"; item: Item }
  | { type: "recent"; entry: RecentSearch };

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setV(value), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return v;
}

/** The current search route's query and scope ("" and catalog elsewhere). */
function useSearchRoute() {
  return useRouterState({
    select: (s) => {
      if (s.location.pathname !== "/search") return { q: "", scope: "catalog" as SearchScope };
      const search = s.location.search as { q?: string; scope?: SearchScope };
      return { q: search.q ?? "", scope: search.scope ?? "catalog" };
    },
    structuralSharing: true,
  });
}

function useHints(term: string, scope: SearchScope, enabled: boolean) {
  return useQuery({
    queryKey: scope === "library" ? keys.librarySearch(term, 3) : keys.catalogSearchHints(term),
    queryFn: async () => {
      if (scope === "library") {
        const c = libraryCards(await SearchService.Library(term, 3));
        return {
          terms: [],
          items: [...c.artists, ...c.albums, ...c.playlists, ...c.songs].slice(0, 6),
        };
      }
      return SearchService.Hints(term);
    },
    enabled: enabled && !!term,
    staleTime: 5 * 60_000,
    placeholderData: (prev) => prev,
    retry: false,
  });
}

function OptionRow({
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
    <li
      id={id}
      role="option"
      aria-selected={active}
      // Keep focus in the field: the pick happens on click.
      onMouseDown={(e) => e.preventDefault()}
      onClick={onPick}
      onMouseMove={onHover}
      className={cn(
        "flex min-h-9 cursor-default items-center gap-3 rounded-md px-2 py-1 text-ui text-ink",
        active && "bg-active",
      )}
    >
      {children}
    </li>
  );
}

function GroupLabel({ children }: { children: ReactNode }) {
  return (
    <li role="presentation" className="px-2 pt-2 pb-1 text-meta text-ink-faint">
      {children}
    </li>
  );
}

/**
 * The top bar's search field: suggestions as you type (completions and top
 * results that open directly), recent searches when empty, Enter to search.
 * "/" or Ctrl+F focuses it from anywhere; ↑/↓ move through the list, Esc
 * closes it, and ↓ with the list closed moves into the results.
 */
export function SearchField() {
  const navigate = useNavigate();
  const pathname = useLocation({ select: (l) => l.pathname });
  const route = useSearchRoute();
  const listId = useId();
  const inputRef = useRef<HTMLInputElement>(null);

  // The draft belongs to the location it was typed at: any navigation hands
  // the field back to the route's query.
  const at = `${pathname}\n${route.q}\n${route.scope}`;
  const [draft, setDraft] = useState({ at, value: route.q });
  const value = draft.at === at ? draft.value : route.q;
  const setValue = (v: string) => setDraft({ at, value: v });

  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const term = useDebounced(value.trim(), HINT_DELAY);
  const scope = route.scope;
  const hints = useHints(term, scope, open);
  const recent = useRecentSearches();
  const { rememberTerm, rememberItem } = useRecentActions();

  const typed = value.trim();
  const options: Option[] = typed
    ? [
        ...(hints.data?.terms ?? []).map((t): Option => ({
          type: "term",
          term: t.term,
          display: t.display,
        })),
        ...(hints.data?.items ?? []).map((item): Option => ({ type: "item", item })),
      ]
    : (recent.data ?? [])
        .filter((r) => r.kind !== "term" || r.scope === scope)
        .slice(0, MAX_RECENT)
        .map((entry): Option => ({ type: "recent", entry }));
  const shown = open && options.length > 0;
  const activeIndex = active < options.length ? active : -1;

  // "/" and Ctrl+F focus the field from anywhere outside a text input.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const typing =
        e.target instanceof HTMLElement && e.target.closest("input, textarea, [contenteditable]");
      if ((e.key === "/" && !typing) || (e.key === "f" && (e.ctrlKey || e.metaKey))) {
        e.preventDefault();
        inputRef.current?.focus();
        inputRef.current?.select();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const close = () => {
    setOpen(false);
    setActive(-1);
  };

  const search = (q: string, inScope: SearchScope = scope) => {
    close();
    const t = q.trim();
    if (t) rememberTerm(t, inScope);
    setDraft({ at, value: t });
    void navigate({
      to: "/search",
      search: { q: t || undefined, scope: inScope === "library" ? "library" : undefined },
      replace: pathname === "/search" && !route.q,
    });
  };

  const openItem = (item: Item) => {
    close();
    inputRef.current?.blur();
    rememberItem(item);
    const link = item.kind === "song" ? undefined : itemLink(item);
    if (link) void navigate(link);
    else void playItem(item);
  };

  const pick = (o: Option) => {
    if (o.type === "term") return search(o.term);
    if (o.type === "item") return openItem(o.item);
    const r = o.entry;
    if (r.kind === "term") return search(r.term, r.scope === "library" ? "library" : "catalog");
    const link = recentLink(r);
    close();
    if (!link) return;
    rememberItem(recentItem(r), r.source);
    void navigate(link);
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const o = activeIndex >= 0 ? options[activeIndex] : undefined;
    if (o) pick(o);
    else search(value);
  };

  const onKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    switch (e.key) {
      case "ArrowDown":
      case "ArrowUp": {
        if (!shown && e.key === "ArrowDown" && pathname === "/search") {
          // The list is closed: ↓ moves into the results.
          const first = document.querySelector<HTMLElement>(
            "#content [data-result], #content [role=row][tabindex='0'], #content [role=listitem] a",
          );
          if (first) {
            e.preventDefault();
            first.focus();
          }
          return;
        }
        e.preventDefault();
        setOpen(true);
        setActive(moveActive(activeIndex, e.key === "ArrowDown" ? 1 : -1, options.length));
        return;
      }
      case "Escape":
        e.preventDefault();
        if (shown) close();
        else inputRef.current?.blur();
        return;
      case "Tab":
        close();
        return;
    }
  };

  const optionId = (i: number) => `${listId}-${i}`;
  const hasTerms = options.some((o) => o.type === "term");
  const firstItem = options.findIndex((o) => o.type === "item");

  return (
    <form
      role="search"
      onSubmit={submit}
      className="relative w-full max-w-[520px]"
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) close();
      }}
    >
      <Search
        aria-hidden
        className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-faint"
        strokeWidth={1.75}
      />
      <Input
        ref={inputRef}
        type="text"
        role="combobox"
        enterKeyHint="search"
        aria-label="Search"
        aria-autocomplete="list"
        aria-expanded={shown}
        aria-controls={listId}
        aria-activedescendant={shown && activeIndex >= 0 ? optionId(activeIndex) : undefined}
        autoComplete="off"
        spellCheck={false}
        value={value}
        onChange={(e) => {
          setValue(e.target.value);
          setOpen(true);
          setActive(-1);
        }}
        onFocus={() => setOpen(true)}
        onClick={() => setOpen(true)}
        onKeyDown={onKeyDown}
        placeholder={scope === "library" ? "Search your library" : "Search Apple Music"}
        className="h-8 border-transparent bg-hover pr-10 pl-9 text-ui shadow-none placeholder:text-ink-faint focus-visible:border-control-line focus-visible:bg-raised focus-visible:ring-0"
      />
      {value ? (
        <button
          type="button"
          aria-label="Clear search"
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            setValue("");
            setActive(-1);
            inputRef.current?.focus();
          }}
          className="absolute top-1/2 right-1.5 grid size-6 -translate-y-1/2 place-items-center rounded-sm text-ink-faint transition-colors duration-100 hover:bg-active hover:text-ink"
        >
          <X aria-hidden className="size-3.5" strokeWidth={1.75} />
        </button>
      ) : (
        <kbd className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 rounded-sm border border-control-line px-1.5 text-meta text-ink-faint">
          /
        </kbd>
      )}
      <ul
        id={listId}
        role="listbox"
        aria-label={typed ? "Suggestions" : "Recent searches"}
        hidden={!shown}
        className="absolute inset-x-0 top-full z-50 mt-1.5 max-h-[min(70vh,520px)] overflow-y-auto rounded-lg border border-line bg-raised p-1.5 shadow-float"
      >
        {!typed && <GroupLabel>Recent searches</GroupLabel>}
        {options.map((o, i) => {
          const common = {
            id: optionId(i),
            active: i === activeIndex,
            onPick: () => pick(o),
            onHover: () => setActive(i),
          };
          if (o.type === "term") {
            const [before, match, after] = splitMatch(o.display, typed);
            return (
              <OptionRow key={`t:${o.term}`} {...common}>
                <Search aria-hidden className="size-4 shrink-0 text-ink-faint" strokeWidth={1.75} />
                <span className="truncate">
                  <span className="text-ink-muted">
                    {before}
                    {match}
                  </span>
                  <span className="font-semibold">{after}</span>
                </span>
              </OptionRow>
            );
          }
          const item = o.type === "item" ? o.item : undefined;
          const entry = o.type === "recent" ? o.entry : undefined;
          const label =
            i === firstItem ? (
              <GroupLabel key="label-items">
                {scope === "library" ? "In your library" : hasTerms ? "Top results" : "Results"}
              </GroupLabel>
            ) : null;
          if (entry?.kind === "term") {
            return (
              <OptionRow key={entry.key} {...common}>
                <Clock aria-hidden className="size-4 shrink-0 text-ink-faint" strokeWidth={1.75} />
                <span className="truncate">{entry.term}</span>
              </OptionRow>
            );
          }
          const it = item ?? recentItem(entry!);
          return [
            label,
            <OptionRow key={entry?.key ?? `i:${it.kind}:${it.id}`} {...common}>
              <Artwork src={it.art.small} size={32} alt="" shape={itemShape(it)} />
              <span className="min-w-0 flex-1">
                <span className="block truncate">{it.name}</span>
                <span className="block truncate text-sub text-ink-muted">{itemMeta(it)}</span>
              </span>
            </OptionRow>,
          ];
        })}
      </ul>
    </form>
  );
}
