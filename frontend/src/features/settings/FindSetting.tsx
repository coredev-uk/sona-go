import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { Search } from "lucide-react";
import { useRef, useState, type KeyboardEvent, type ReactNode } from "react";

import { offersLanguages } from "@/shared/i18n";
import { useCapabilities } from "@/shared/lib/capabilities";
import { Input } from "@/shared/ui/input";

import { findSettings, sectionTitle, settingIndex, type SettingEntry } from "./find";
import { useLanguageOptions } from "./language-queries";
import { useSettings } from "./queries";

/** Results shown at once; a longer query narrows the rest. */
const MAX_RESULTS = 12;

/**
 * Find a setting: a field over the section list. While it holds a query
 * the list gives way to the settings it finds (titles, groups, sections
 * and a few other words for each); choosing one opens its section and puts
 * focus on its control. ↓ goes from the field into the results, ↑/↓,
 * Home/End move between them, Enter in the field takes the first, Escape
 * clears the query. The count is announced in a status region.
 */
export function FindSetting({
  title,
  children,
  onPick,
}: {
  /** The page's title, set beside the field below 960px and above it wider. */
  title: ReactNode;
  /** The section list, shown while the field is empty. */
  children: ReactNode;
  onPick: (entry: SettingEntry) => void;
}) {
  const { t, i18n } = useLingui();
  const [query, setQuery] = useState("");
  const caps = useCapabilities();
  const settings = useSettings().data;
  const language = settings?.language ?? "";
  const options = useLanguageOptions(language);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);

  const q = query.trim();
  const found = q
    ? findSettings(
        settingIndex(),
        q,
        { caps, settings, languageRow: !!options && offersLanguages(options, language) },
        i18n.locale,
      )
    : [];
  const shown = found.slice(0, MAX_RESULTS);
  const count = found.length;
  const status = !q
    ? ""
    : count
      ? plural(count, { one: "# setting found", other: "# settings found" })
      : t`No setting matches “${q}”.`;

  const pick = (e: SettingEntry) => {
    setQuery("");
    onPick(e);
  };
  const results = () => [...(list.current?.querySelectorAll<HTMLButtonElement>("button") ?? [])];

  const onFieldKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown" && shown.length) {
      e.preventDefault();
      results()[0]?.focus();
    } else if (e.key === "Enter" && shown.length) {
      e.preventDefault();
      pick(shown[0]);
    } else if (e.key === "Escape" && query) {
      e.preventDefault();
      e.stopPropagation();
      setQuery("");
    }
  };
  const onListKey = (e: KeyboardEvent<HTMLUListElement>) => {
    const all = results();
    const at = all.indexOf(document.activeElement as HTMLButtonElement);
    if (at < 0) return;
    let next: HTMLElement | undefined;
    if (e.key === "ArrowDown") next = all[Math.min(at + 1, all.length - 1)];
    else if (e.key === "ArrowUp") next = at === 0 ? (input.current ?? undefined) : all[at - 1];
    else if (e.key === "Home") next = all[0];
    else if (e.key === "End") next = all[all.length - 1];
    else if (e.key === "Escape") {
      setQuery("");
      next = input.current ?? undefined;
    } else return;
    e.preventDefault();
    e.stopPropagation();
    next?.focus();
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="mb-2 flex flex-col gap-5 max-[960px]:mb-0 max-[960px]:flex-row max-[960px]:items-center max-[960px]:justify-between">
        {title}
        <div role="search" className="relative max-[960px]:w-72">
          <Search
            aria-hidden
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-faint"
            strokeWidth={1.75}
          />
          <Input
            ref={input}
            type="search"
            aria-label={t`Find a setting`}
            placeholder={t`Find a setting`}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onFieldKey}
            className="h-8 pl-9"
          />
        </div>
      </div>
      {/* Exists before any message: WebKitGTK announces an inserted child, not changed text. */}
      <div role="status" className="sr-only">
        {status && <p key={status}>{status}</p>}
      </div>
      {!q ? (
        children
      ) : shown.length ? (
        <ul
          ref={list}
          aria-label={t`Settings found`}
          onKeyDown={onListKey}
          className="flex flex-col"
        >
          {shown.map((e) => {
            const section = sectionTitle(e.section);
            return (
              <li key={e.id}>
                <button
                  type="button"
                  onClick={() => pick(e)}
                  className="flex w-full flex-col items-start rounded-md px-3 py-1.5 text-left transition-colors duration-100 hover:bg-hover focus-visible:bg-hover"
                >
                  <span className="text-ui text-ink">{e.title}</span>
                  <span className="text-meta text-ink-muted">
                    {e.group === section ? section : `${section} · ${e.group}`}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="px-3 py-1.5 text-sub text-ink-muted">
          <Trans>No setting matches “{q}”.</Trans>
        </p>
      )}
    </div>
  );
}
