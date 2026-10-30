import type { RecentSearch } from "@bindings/services";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Clock, X } from "lucide-react";

import { Artwork } from "@/shared/components/Artwork";
import { Shelf } from "@/shared/components/Shelf";
import { itemShape } from "@/shared/items/links";
import { run } from "@/shared/lib/run";
import { Button } from "@/shared/ui/button";

import { itemMeta } from "./groups";
import { useRecentActions, useRecentSearches } from "./queries";
import { isLibraryEntry, recentItem, recentLink } from "./recent";
import type { SearchScope } from "./search-params";

const MAX_SHOWN = 12;
const ROW_CLASS =
  "flex min-w-0 flex-1 items-center gap-3 rounded-md px-2 py-1.5 focus-visible:outline-none";

function Entry({ entry, scope }: { entry: RecentSearch; scope: SearchScope }) {
  const { rememberTerm, rememberItem, forget } = useRecentActions();
  const { t } = useLingui();
  const row = ROW_CLASS;
  let body;
  if (entry.kind === "term") {
    body = (
      <Link
        to="/search"
        search={{ q: entry.term, scope: scope === "library" ? "library" : undefined }}
        data-result
        className={row}
        onClick={() => rememberTerm(entry.term, scope)}
      >
        <span className="grid size-10 shrink-0 place-items-center rounded-sm bg-hover">
          <Clock aria-hidden className="size-4 text-ink-faint" strokeWidth={1.75} />
        </span>
        <span className="truncate text-ui text-ink">{entry.term}</span>
      </Link>
    );
  } else {
    const item = recentItem(entry);
    const link = recentLink(entry);
    if (!link) return null;
    body = (
      <Link {...link} data-result className={row} onClick={() => rememberItem(item, entry.source)}>
        <Artwork src={entry.art} size={40} alt="" shape={itemShape(item)} />
        <span className="min-w-0">
          <span className="block truncate text-ui text-ink">{entry.name}</span>
          <span className="block truncate text-sub text-ink-muted">{itemMeta(item)}</span>
        </span>
      </Link>
    );
  }
  const label = entry.kind === "term" ? entry.term : entry.name;
  return (
    <li className="group/recent flex min-w-0 items-center rounded-md pr-1 outline-offset-[-2px] transition-colors duration-100 focus-within:bg-hover hover:bg-hover has-[[data-result]:focus-visible]:outline-2 has-[[data-result]:focus-visible]:outline-ring has-[[data-result]:focus-visible]:outline-solid">
      {body}
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={t`Remove ${label} from recent searches`}
        className="text-ink-muted opacity-0 group-focus-within/recent:opacity-100 group-hover/recent:opacity-100"
        onClick={() => void run(t`Couldn't remove the search`, forget(entry.key))}
      >
        <X strokeWidth={1.75} />
      </Button>
    </li>
  );
}

/**
 * The searches and opened results remembered on this computer, for the
 * scope on screen: terms searched there, and items from it.
 */
export function RecentSearches({ scope }: { scope: SearchScope }) {
  const { t } = useLingui();
  const recent = useRecentSearches();
  const { clear } = useRecentActions();
  const entries = (recent.data ?? [])
    .filter((r) =>
      r.kind === "term" ? r.scope === scope : isLibraryEntry(r) === (scope === "library"),
    )
    .slice(0, MAX_SHOWN);
  if (!entries.length) return null;
  return (
    <Shelf
      title={t`Recent searches`}
      actions={
        <button
          type="button"
          className="-mr-1 inline-flex h-7 items-center rounded-md px-1.5 text-sub text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink"
          onClick={() => void run(t`Couldn't clear recent searches`, clear())}
        >
          <Trans>Clear</Trans>
        </button>
      }
    >
      <ul
        aria-label={t`Recent searches`}
        className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-x-6 gap-y-1"
      >
        {entries.map((e) => (
          <Entry key={e.key} entry={e} scope={scope} />
        ))}
      </ul>
    </Shelf>
  );
}
