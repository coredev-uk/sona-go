import type { Item, LibrarySearch, SearchSection } from "@bindings/services";
import { getRouteApi } from "@tanstack/react-router";
import { useCallback, useMemo, useState } from "react";

import { hasAccount, useAuth } from "@/features/auth";
import { playSongs } from "@/features/player";
import { EmptyState } from "@/shared/components/EmptyState";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { Shelf } from "@/shared/components/Shelf";
import { ItemGrid } from "@/shared/items/ItemGrid";
import { ItemShelf } from "@/shared/items/ItemShelf";
import { callFailure } from "@/shared/lib/errors";
import { TrackList } from "@/shared/tracks/TrackList";

import { groupTitle, hasSeeAll } from "./groups";
import { ItemRow } from "./ItemRow";
import {
  libraryCards,
  useBrowse,
  useCatalogSearch,
  useLibrarySearch,
  useRecentActions,
} from "./queries";
import { RecentSearches } from "./RecentSearches";
import { ScopeSwitch } from "./ScopeSwitch";
import type { SearchScope } from "./search-params";

const route = getRouteApi("/search");

/** Songs shown before "See all". */
const SONG_ROWS = 8;
/** Top results shown as rows. */
const TOP_ROWS = 9;
/** Library matches per kind. */
const LIBRARY_PER_KIND = 25;

function CatalogSection({
  q,
  section,
  onOpen,
}: {
  q: string;
  section: SearchSection;
  onOpen: (item: Item) => void;
}) {
  const { group, items, next } = section;
  const title = groupTitle(group);
  const songs = useMemo(
    () =>
      group === "song" ? items.flatMap((i) => (i.song ? [i.song] : [])).slice(0, SONG_ROWS) : [],
    [group, items],
  );
  const onPlay = useCallback(
    (i: number) => {
      onOpen(items[i]);
      playSongs(songs, i, "search");
    },
    [items, songs, onOpen],
  );

  if (group === "top") {
    return (
      <Shelf title={title}>
        <ul
          aria-label={`${title} for ${q}`}
          className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-x-6 gap-y-1"
        >
          {items.slice(0, TOP_ROWS).map((item) => (
            <li key={`${item.kind}:${item.id}`} className="min-w-0">
              <ItemRow item={item} size={48} onOpen={onOpen} />
            </li>
          ))}
        </ul>
      </Shelf>
    );
  }
  const seeAll = hasSeeAll(group, next, group === "song" ? SONG_ROWS : items.length, items.length);
  if (group === "song") {
    return (
      <Shelf
        title={title}
        more={seeAll ? { to: "/search/$group", params: { group }, search: { q } } : undefined}
      >
        <TrackList
          songs={songs}
          columns={{ number: "row", artwork: true, artist: true, album: true }}
          label={`Songs matching ${q}`}
          onPlay={onPlay}
        />
      </Shelf>
    );
  }
  return (
    <ItemShelf
      title={title}
      items={items}
      more={seeAll ? { to: "/search/$group", params: { group }, search: { q } } : undefined}
      onOpen={onOpen}
    />
  );
}

function LoadingRows() {
  return (
    <div aria-busy aria-label="Searching" className="flex flex-col gap-3">
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="flex h-11 items-center gap-3 px-2">
          <div className="size-8 rounded-sm bg-hover" />
          <div className="h-3.5 w-1/3 rounded-sm bg-hover" />
        </div>
      ))}
    </div>
  );
}

function CatalogResults({ q }: { q: string }) {
  const auth = useAuth();
  const res = useCatalogSearch(q);
  const { rememberTerm, rememberItem } = useRecentActions();
  // Opening a result remembers the search that found it, and the result.
  const onOpen = useCallback(
    (item: Item) => {
      rememberTerm(q, "catalog");
      rememberItem(item);
    },
    [q, rememberTerm, rememberItem],
  );

  if (res.isPending) return <LoadingRows />;
  if (res.error) {
    const { code } = callFailure(res.error);
    if (!hasAccount(auth.data?.state) && (code === "not_found" || code === "unauthorized")) {
      return (
        <EmptyState
          title="Sign in to search Apple Music"
          action={{ label: "Sign in", to: "/settings", search: { section: "account" } }}
        >
          Searching the catalogue needs your account's storefront. Your library on this computer can
          be searched either way.
        </EmptyState>
      );
    }
    return (
      <ErrorBlock
        title="Couldn't search Apple Music"
        error={res.error}
        onRetry={() => void res.refetch()}
        retrying={res.isRefetching}
      />
    );
  }
  if (!res.data.sections.length) {
    return (
      <EmptyState title={`Nothing on Apple Music for “${q}”`}>
        Check the spelling, try fewer words, or search your library instead.
      </EmptyState>
    );
  }
  return (
    <div aria-busy={res.isPlaceholderData || undefined}>
      {res.data.sections.map((s) => (
        <CatalogSection key={s.group} q={q} section={s} onOpen={onOpen} />
      ))}
    </div>
  );
}

function LibraryResults({ q }: { q: string }) {
  const res = useLibrarySearch(q, LIBRARY_PER_KIND);
  const [allSongs, setAllSongs] = useState(false);
  const { rememberTerm, rememberItem } = useRecentActions();
  const onOpen = useCallback(
    (item: Item) => {
      rememberTerm(q, "library");
      rememberItem(item);
    },
    [q, rememberTerm, rememberItem],
  );
  const songs = res.data?.songs;
  const onPlay = useCallback(
    (i: number) => {
      if (!songs) return;
      rememberTerm(q, "library");
      playSongs(songs, i, "search");
    },
    [songs, q, rememberTerm],
  );

  if (res.error) {
    return (
      <ErrorBlock
        title="Couldn't search your library"
        error={res.error}
        onRetry={() => void res.refetch()}
      />
    );
  }
  if (!res.data) return <LoadingRows />;
  const r: LibrarySearch = res.data;
  if (!r.songs.length && !r.albums.length && !r.artists.length && !r.playlists.length) {
    return (
      <EmptyState title={`Nothing in your library for “${q}”`}>
        Only what is synced to this computer is searched. Try Apple Music for everything else.
      </EmptyState>
    );
  }
  const cards = libraryCards(r);
  return (
    <>
      {r.songs.length > 0 && (
        <Shelf
          title="Songs"
          actions={
            r.songs.length > SONG_ROWS && (
              <button
                type="button"
                aria-expanded={allSongs}
                onClick={() => setAllSongs((o) => !o)}
                className="-mr-1 inline-flex h-7 items-center rounded-md px-1.5 text-sub text-ink-muted transition-colors duration-100 hover:bg-hover hover:text-ink"
              >
                {allSongs ? "Show fewer" : `Show all ${r.songs.length}`}
              </button>
            )
          }
        >
          <TrackList
            songs={allSongs ? r.songs : r.songs.slice(0, SONG_ROWS)}
            columns={{ number: "row", artwork: true, artist: true, album: true }}
            label={`Library songs matching ${q}`}
            onPlay={onPlay}
          />
        </Shelf>
      )}
      {r.artists.length > 0 && <ItemShelf title="Artists" items={cards.artists} onOpen={onOpen} />}
      {r.albums.length > 0 && <ItemShelf title="Albums" items={cards.albums} onOpen={onOpen} />}
      {r.playlists.length > 0 && (
        <ItemShelf title="Playlists" items={cards.playlists} onOpen={onOpen} />
      )}
    </>
  );
}

function Browse() {
  const res = useBrowse(true);
  const { rememberItem } = useRecentActions();
  if (res.error) {
    return (
      <Shelf title="Browse categories">
        <ErrorBlock
          title="Couldn't load the categories"
          error={res.error}
          onRetry={() => void res.refetch()}
          retrying={res.isRefetching}
        />
      </Shelf>
    );
  }
  const items = res.data ?? [];
  return (
    <Shelf title="Browse categories">
      {res.isPending ? (
        <div aria-busy className="grid grid-cols-[repeat(auto-fill,minmax(160px,1fr))] gap-6">
          {Array.from({ length: 12 }, (_, i) => (
            <div key={i} className="aspect-square rounded-sm bg-hover" />
          ))}
        </div>
      ) : (
        <ItemGrid items={items} label="Browse categories" onOpen={rememberItem} />
      )}
    </Shelf>
  );
}

export function SearchPage() {
  const { q, scope = "catalog" } = route.useSearch();
  const navigate = route.useNavigate();
  const setScope = (s: SearchScope) =>
    void navigate({ search: { q, scope: s === "library" ? "library" : undefined }, replace: true });
  const auth = useAuth();
  const signedIn = hasAccount(auth.data?.state);

  return (
    <>
      <PageHeader
        title={q ? `“${q}”` : "Search"}
        actions={<ScopeSwitch scope={scope} onChange={setScope} />}
      />
      {!q ? (
        <>
          <RecentSearches scope={scope} />
          {scope === "library" ? (
            <EmptyState title="Search your library">
              Songs, albums, artists and playlists synced to this computer, matched as you type each
              word.
            </EmptyState>
          ) : signedIn || auth.data ? (
            <Browse />
          ) : null}
        </>
      ) : scope === "library" ? (
        <LibraryResults q={q} />
      ) : (
        <CatalogResults q={q} />
      )}
    </>
  );
}
