import { LibraryService, type Album, type ArtistEntry, type Song } from "@bindings/services";
import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { ChevronLeft, Search } from "lucide-react";
import {
  memo,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";

import { releaseTitle } from "@/features/artist";
import { playSongs, shuffleSongs } from "@/features/player";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { CoverGrid, type CoverItem } from "@/shared/components/CoverGrid";
import { ErrorBlock } from "@/shared/components/ErrorBlock";
import { PageHeader } from "@/shared/components/PageHeader";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { keys } from "@/shared/lib/keys";
import { albumTile } from "@/shared/lib/tiles";
import { useContentHeight } from "@/shared/lib/useContentHeight";
import { useElementWidth } from "@/shared/lib/useCoverColumns";
import { usePaneVirtualizer } from "@/shared/lib/usePaneVirtualizer";
import { cn } from "@/shared/lib/utils";
import { TrackList } from "@/shared/tracks/TrackList";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";

import {
  ARTIST_ROW,
  LETTER_ROW,
  filterArtists,
  libraryLine,
  listRows,
  type ArtistListRow,
  type IndexedArtist,
} from "./artists";
import { useLibraryGate } from "./gate";
import { useArtistAlbums, useArtistIndex } from "./queries";

const route = getRouteApi("/library/artists");

/** Under this width of page the list and the detail take turns instead of sitting side by side. */
const NARROW = 880;
/** Artists a page-step key moves by. */
const PAGE_STEP = 10;

const optionId = (id: string) => `artist-option-${id}`;

/** One artist in the list: round portrait, name, and what the library holds of them. */
const ArtistOption = memo(function ArtistOption({
  artist,
  position,
  count,
  selected,
  onPick,
}: {
  artist: ArtistEntry;
  position: number;
  count: number;
  selected: boolean;
  onPick: (id: string) => void;
}) {
  const line = libraryLine(artist.albums, artist.songs);
  return (
    <div
      role="option"
      id={optionId(artist.id)}
      aria-selected={selected}
      aria-posinset={position + 1}
      aria-setsize={count}
      onClick={() => onPick(artist.id)}
      className={cn(
        "flex h-11 cursor-default items-center gap-3 rounded-md px-3 transition-colors duration-100",
        selected ? "bg-active" : "hover:bg-hover",
      )}
    >
      <Artwork src={artist.art} size={32} alt="" shape="round" />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-ui font-medium text-ink">{artist.name}</span>
        {line && <span className="block truncate text-meta text-ink-muted">{line}</span>}
      </span>
    </div>
  );
});

/** Rows where an artist will be, while the library is read. */
function ListSkeleton() {
  return (
    <div aria-hidden className="px-3">
      {Array.from({ length: 14 }, (_, i) => (
        <div key={i} className="flex h-12 items-center gap-3 px-3">
          <div className="size-8 rounded-full bg-hover" />
          <div className="flex flex-col gap-1.5">
            <div className="h-3.5 w-32 rounded-sm bg-hover" />
            <div className="h-3 w-20 rounded-sm bg-hover" />
          </div>
        </div>
      ))}
    </div>
  );
}

interface ArtistListProps {
  total: number;
  rows: readonly ArtistListRow[];
  matches: number;
  query: string;
  onQuery: (query: string) => void;
  /** The row the list marks: the open artist beside the detail, the cursor otherwise. */
  highlightId: string | undefined;
  onPick: (id: string) => void;
  onStep: (by: number) => void;
  onEnds: (end: "first" | "last") => void;
  onOpen: () => void;
  inputRef: RefObject<HTMLInputElement | null>;
  className?: string;
  loading: boolean;
}

/**
 * The searchable artist list: a title, the filter, then the artists A to Z
 * under their letters (virtualised in its own scroll box). The filter and
 * the list share one keyboard: ↑/↓ move, Enter or → go into the detail,
 * typing in the list types in the filter.
 */
function ArtistList({
  total,
  rows,
  matches,
  query,
  onQuery,
  highlightId,
  onPick,
  onStep,
  onEnds,
  onOpen,
  inputRef,
  className,
  loading,
}: ArtistListProps) {
  const { t, i18n } = useLingui();
  const [listEl, setListEl] = useState<HTMLDivElement | null>(null);
  const sizeAt = useMemo(
    () => (i: number) => (rows[i]?.kind === "letter" ? LETTER_ROW : ARTIST_ROW),
    [rows],
  );
  const { virtualizer, items, totalSize } = usePaneVirtualizer(listEl, rows.length, sizeAt);
  const highlightRow = useMemo(
    () => rows.findIndex((r) => r.kind === "artist" && r.artist.entry.id === highlightId),
    [rows, highlightId],
  );
  useEffect(() => {
    if (highlightRow >= 0) virtualizer.scrollToIndex(highlightRow, { align: "auto" });
  }, [virtualizer, highlightRow]);

  const onKeyDown = (e: KeyboardEvent<HTMLElement>, field: boolean) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    const input = e.currentTarget instanceof HTMLInputElement ? e.currentTarget : null;
    switch (e.key) {
      case "ArrowDown":
      case "ArrowUp":
        e.preventDefault();
        onStep(e.key === "ArrowDown" ? 1 : -1);
        return;
      case "PageDown":
      case "PageUp":
        e.preventDefault();
        onStep(e.key === "PageDown" ? PAGE_STEP : -PAGE_STEP);
        return;
      case "Home":
      case "End":
        // The field keeps Home and End for its caret.
        if (field) return;
        e.preventDefault();
        onEnds(e.key === "Home" ? "first" : "last");
        return;
      case "Enter":
        e.preventDefault();
        onOpen();
        return;
      case "ArrowRight":
        // In the field → goes into the detail only from the end of the text.
        if (
          input &&
          (input.selectionStart !== input.value.length || input.selectionEnd !== input.value.length)
        )
          return;
        e.preventDefault();
        onOpen();
        return;
      case "Escape":
        if (query) {
          e.preventDefault();
          onQuery("");
          inputRef.current?.focus();
        }
        return;
    }
    // A letter typed on the list goes to the filter.
    if (!field && e.key.length === 1) {
      e.preventDefault();
      onQuery(query + e.key);
      inputRef.current?.focus();
    }
  };

  // Only an option that is on the page can be the active descendant.
  const everyone = plural(total, { one: "# artist", other: "# artists" });
  const found = matches.toLocaleString(i18n.locale);
  const active =
    highlightId && items.some((it) => it.index === highlightRow)
      ? optionId(highlightId)
      : undefined;
  return (
    <div className={cn("sticky top-0 flex h-(--pane-h) shrink-0 flex-col", className)}>
      <div className="px-6 pt-7 pb-3">
        <h1 className="truncate text-page text-ink">
          <Trans>Artists</Trans>
        </h1>
        <p className="mt-1 min-h-[18px] text-sub text-ink-muted num">
          {total ? (query.trim() ? t`${found} of ${everyone}` : everyone) : null}
        </p>
        <div className="relative mt-4">
          <Search
            aria-hidden
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-faint"
            strokeWidth={1.75}
          />
          <Input
            ref={inputRef}
            type="search"
            role="combobox"
            aria-label={t`Filter artists`}
            aria-expanded
            aria-haspopup="listbox"
            aria-controls="artist-list"
            aria-autocomplete="list"
            aria-activedescendant={active}
            placeholder={t`Filter artists`}
            value={query}
            autoComplete="off"
            spellCheck={false}
            onChange={(e) => onQuery(e.target.value)}
            onKeyDown={(e) => onKeyDown(e, true)}
            className="h-8 pl-9"
          />
        </div>
        <div role="status">
          {query && (
            <p key={`${query}-${matches}`} className="sr-only">
              {matches
                ? plural(matches, { one: "# artist matches", other: "# artists match" })
                : t`No artist matches “${query}”`}
            </p>
          )}
        </div>
      </div>
      {loading ? (
        <ListSkeleton />
      ) : matches === 0 ? (
        <p aria-hidden className="px-6 py-4 text-sub text-ink-muted">
          <Trans>No artist matches “{query}”.</Trans>
        </p>
      ) : (
        <div
          ref={setListEl}
          id="artist-list"
          role="listbox"
          tabIndex={0}
          aria-label={t`Artists in your library`}
          aria-activedescendant={active}
          onKeyDown={(e) => onKeyDown(e, false)}
          className="min-h-0 flex-1 overflow-y-auto px-3 pb-6 focus-visible:outline-offset-[-2px]"
        >
          <div className="relative" style={{ height: totalSize }}>
            {items.map((it) => {
              const row = rows[it.index];
              return (
                <div
                  key={it.key}
                  className="absolute inset-x-0 top-0"
                  style={{ height: it.size, transform: `translateY(${it.start}px)` }}
                >
                  {row.kind === "letter" ? (
                    <div
                      role="presentation"
                      className="flex h-full items-end px-3 pb-1 text-meta font-semibold text-ink-muted"
                    >
                      {row.letter}
                    </div>
                  ) : (
                    <ArtistOption
                      artist={row.artist.entry}
                      position={row.position}
                      count={matches}
                      selected={row.artist.entry.id === highlightId}
                      onPick={onPick}
                    />
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

/** An album's tile: a single's or EP's title without Apple's suffix, its kind with the year. */
function releaseTile(a: Album, kindLabels: { single: string; ep: string }) {
  const { title, kind } = releaseTitle(a.name);
  const tile = albumTile(a, "year");
  const label =
    kind === "ep" ? kindLabels.ep : kind === "single" || a.single ? kindLabels.single : "";
  return label
    ? ({
        ...tile,
        title,
        subtitle: [tile.subtitle, label].filter(Boolean).join(" · "),
      } satisfies CoverItem)
    : tile;
}

/** The detail's header and grid where they will be, while an artist's albums are read. */
function AlbumsSkeleton() {
  return (
    <div
      aria-hidden
      className="grid gap-x-6 gap-y-7"
      style={{ gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))" }}
    >
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i}>
          <div className="aspect-square w-full rounded-sm bg-hover" />
          <div className="mt-2 h-3.5 w-3/4 rounded-sm bg-hover" />
          <div className="mt-1.5 h-3 w-1/3 rounded-sm bg-hover" />
        </div>
      ))}
    </div>
  );
}

/** An artist whose library holds songs but no album: their songs, where the albums would be. */
function SongsOnly({
  artist,
  Group,
  onPlay,
}: {
  artist: ArtistEntry;
  Group: "h2" | "h3";
  onPlay: (songs: Song[], index: number) => void;
}) {
  const { t } = useLingui();
  const name = artist.name;
  const q = useQuery({
    queryKey: keys.libraryArtist(artist.id),
    queryFn: () => LibraryService.Artist(artist.id),
    enabled: artist.songs > 0,
  });
  if (artist.songs === 0) {
    return (
      <p className="max-w-[60ch] text-ui text-ink-muted">
        <Trans>Add music by {name} to collect it here.</Trans>
      </p>
    );
  }
  if (q.error) {
    return (
      <ErrorBlock
        title={t`Couldn't read this artist's songs from this computer`}
        error={q.error}
        onRetry={() => void q.refetch()}
        retrying={q.isRefetching}
      />
    );
  }
  const songs = q.data?.songs;
  if (!songs) return <AlbumsSkeleton />;
  return (
    <section aria-labelledby="artist-songs">
      <Group id="artist-songs" className="mb-4 text-heading text-ink">
        <Trans>Songs in your library</Trans>
      </Group>
      <TrackList
        songs={songs}
        columns={{ number: "row", artwork: true, album: true }}
        label={t`Songs by ${name} in your library`}
        onPlay={(i) => onPlay(songs, i)}
      />
    </section>
  );
}

interface ArtistDetailProps {
  artist: ArtistEntry;
  /** Narrow windows: the way back to the list. */
  onBack?: () => void;
  backRef: RefObject<HTMLButtonElement | null>;
  onLeave: () => void;
}

/**
 * The open artist: portrait, name, what the library holds, play and
 * shuffle, the way to their full page, then the albums in the library as a
 * grid of covers, newest release first, singles and EPs under their own
 * heading. Only what Sona holds on this computer: the catalogue is one
 * link away.
 */
function ArtistDetail({ artist, onBack, backRef, onLeave }: ArtistDetailProps) {
  const { t } = useLingui();
  const qc = useQueryClient();
  const q = useArtistAlbums(artist.id);
  const kindLabels = { single: t`Single`, ep: t`EP` };
  const albums = q.data?.albums ?? [];
  // Go decides what is a single or EP (library.IsSingleRelease).
  const full = albums.filter((a) => !a.single);
  const releases = albums.filter((a) => a.single);
  const name = artist.name;
  // Alone in a narrow window, where the list and its h1 are out of the way, the artist is the page.
  const Name = onBack ? "h1" : "h2";
  const Group = onBack ? "h2" : "h3";

  /** Every song of the artist in the library, read only when asked to play. */
  const songs = () =>
    qc.fetchQuery({
      queryKey: keys.libraryArtist(artist.id),
      queryFn: () => LibraryService.Artist(artist.id),
      staleTime: 30_000,
    });
  const from = { kind: "artist", id: artist.id };

  const line = [t`Artist`, libraryLine(artist.albums, artist.songs)].filter(Boolean).join(" · ");
  return (
    <div
      data-detail={artist.id}
      onKeyDown={(e) => {
        if (e.key === "Escape" && !e.defaultPrevented) onLeave();
      }}
      className="min-w-0 flex-1 px-8 pt-7 pb-12 max-[960px]:px-6"
    >
      {onBack && (
        <Button ref={backRef} variant="ghost" size="sm" onClick={onBack} className="mb-4 -ml-3">
          <ChevronLeft strokeWidth={1.75} />
          <Trans>Artists</Trans>
        </Button>
      )}
      <header className="flex items-end gap-6">
        <Artwork src={artist.art} size={96} alt="" shape="round" eager />
        <div className="min-w-0 flex-1 pb-0.5">
          <Name id="artist-detail-name" className="line-clamp-2 text-display text-balance text-ink">
            {name}
          </Name>
          <p className="mt-1 text-meta text-ink-muted">{line}</p>
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <PlayButtons
              label={name}
              disabled={artist.songs === 0}
              onPlay={() => void songs().then((d) => playSongs(d.songs, 0, from))}
              onShuffle={() => void songs().then((d) => shuffleSongs(d.songs, from))}
            />
            <Button asChild variant="outline" size="sm">
              <Link to="/artist/$id" params={{ id: artist.catalogId || artist.id }}>
                <Trans>Full artist page</Trans>
              </Link>
            </Button>
            {q.data && (
              <ActionMenu kind="artist" target={q.data.artist} name={name} variant="header" />
            )}
          </div>
        </div>
      </header>
      <div className="mt-8 flex flex-col gap-10">
        {q.error ? (
          <ErrorBlock
            title={t`Couldn't read this artist's albums from this computer`}
            error={q.error}
            onRetry={() => void q.refetch()}
            retrying={q.isRefetching}
          />
        ) : q.isPending ? (
          <AlbumsSkeleton />
        ) : albums.length === 0 ? (
          <SongsOnly
            artist={artist}
            Group={Group}
            onPlay={(songs, i) => playSongs(songs, i, from)}
          />
        ) : (
          <>
            {full.length > 0 && (
              <section aria-labelledby="artist-albums">
                <Group id="artist-albums" className="mb-4 text-heading text-ink">
                  <Trans>Albums</Trans>
                </Group>
                <CoverGrid
                  items={full.map((a) => releaseTile(a, kindLabels))}
                  label={t`Albums by ${name} in your library`}
                />
              </section>
            )}
            {releases.length > 0 && (
              <section aria-labelledby="artist-singles">
                <Group id="artist-singles" className="mb-4 text-heading text-ink">
                  <Trans>Singles and EPs</Trans>
                </Group>
                <CoverGrid
                  items={releases.map((a) => releaseTile(a, kindLabels))}
                  label={t`Singles and EPs by ${name} in your library`}
                />
              </section>
            )}
          </>
        )}
      </div>
    </div>
  );
}

/**
 * Focus goes to artist `id`'s first album once their detail is up, or to
 * its first control while the albums are still being read.
 */
function focusDetail(id: string) {
  let tries = 0;
  const go = () => {
    const detail = document.querySelector<HTMLElement>(`[data-detail="${CSS.escape(id)}"]`);
    const tile = detail?.querySelector<HTMLElement>("[data-opener]");
    if (tile) tile.focus();
    else if (tries++ < 30) requestAnimationFrame(go);
    else detail?.querySelector<HTMLElement>("a[href], button:not(:disabled)")?.focus();
  };
  go();
}

/**
 * Master and detail: the artists on the left, filtered as you type, the
 * open artist's library albums on the right. The open artist is the URL's
 * `?artist=`, so Back, Forward and reload keep it; a window too narrow for
 * both shows the list, then the artist with a way back.
 */
function ArtistsOverview({ artists }: { artists: IndexedArtist[] | undefined }) {
  const search = route.useSearch();
  const navigate = route.useNavigate();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  useContentHeight(rootRef);
  const width = useElementWidth(rootRef);
  const narrow = width > 0 && width < NARROW;

  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState<string>();
  const matches = useMemo(() => filterArtists(artists ?? [], query), [artists, query]);
  const rows = useMemo(() => listRows(matches, !query.trim()), [matches, query]);
  const position = useMemo(() => new Map(matches.map((a, i) => [a.entry.id, i])), [matches]);
  const byId = useMemo(() => new Map(artists?.map((a) => [a.entry.id, a.entry])), [artists]);

  // The URL's artist, when it is in the library. Beside the list, the open
  // artist is also one the filter shows: otherwise the first match stands in.
  const urlId = search.artist && byId.has(search.artist) ? search.artist : undefined;
  const openId = narrow ? urlId : urlId && position.has(urlId) ? urlId : matches[0]?.entry.id;
  // In the list alone the cursor marks a row; with none, the artist last opened.
  const cursorId =
    cursor && position.has(cursor) ? cursor : urlId && position.has(urlId) ? urlId : undefined;
  const highlightId = narrow ? cursorId : openId;
  // Beside the list a walk down it stays smooth: the detail follows when the keys rest.
  const deferredId = useDeferredValue(openId);
  const shown = narrow ? openId : deferredId;
  const detail = shown ? byId.get(shown) : undefined;

  // Opened from the list in this window, so Back returns to it.
  const fromList = useRef(false);
  const open = (id: string, replace: boolean) => {
    fromList.current = !replace;
    void navigate({ search: { artist: id }, replace });
  };
  const pick = (id: string) => {
    setCursor(id);
    open(id, false);
    // Keep the keys on the list after a click.
    if (!narrow) document.getElementById("artist-list")?.focus({ preventScroll: true });
  };
  const step = (by: number) => {
    if (!matches.length) return;
    const at = highlightId ? (position.get(highlightId) ?? -1) : -1;
    const to =
      at < 0
        ? by > 0
          ? 0
          : matches.length - 1
        : Math.min(Math.max(at + by, 0), matches.length - 1);
    const id = matches[to].entry.id;
    if (narrow) setCursor(id);
    else open(id, true);
  };
  const ends = (end: "first" | "last") => {
    if (!matches.length) return;
    const id = matches[end === "first" ? 0 : matches.length - 1].entry.id;
    if (narrow) setCursor(id);
    else open(id, true);
  };
  const enter = () => {
    const id = narrow ? (highlightId ?? matches[0]?.entry.id) : openId;
    if (!id) return;
    if (narrow) open(id, false);
    else {
      if (id !== urlId) open(id, true);
      focusDetail(id);
    }
  };
  const back = () => {
    if (fromList.current) window.history.back();
    else void navigate({ search: {}, replace: true });
  };

  // A narrow window changes view: focus follows to the detail's way back, and returns to the filter.
  const inDetail = narrow && !!urlId;
  const wasDetail = useRef<boolean>(undefined);
  useEffect(() => {
    if (wasDetail.current !== undefined && wasDetail.current !== inDetail) {
      (inDetail ? backRef : inputRef).current?.focus();
    }
    wasDetail.current = inDetail;
  }, [inDetail]);

  return (
    <div
      ref={rootRef}
      className="-mx-8 -mt-7 -mb-12 flex min-h-(--pane-h) items-start max-[960px]:-mx-6"
    >
      <ArtistList
        total={artists?.length ?? 0}
        rows={rows}
        matches={matches.length}
        query={query}
        onQuery={setQuery}
        highlightId={highlightId}
        onPick={pick}
        onStep={step}
        onEnds={ends}
        onOpen={enter}
        inputRef={inputRef}
        loading={!artists}
        className={cn(narrow ? "w-full" : "w-80 border-r border-line", inDetail && "hidden")}
      />
      {detail && (!narrow || urlId) && (
        <ArtistDetail
          key={detail.id}
          artist={detail}
          onBack={narrow ? back : undefined}
          backRef={backRef}
          onLeave={() => (narrow ? backRef : inputRef).current?.focus()}
        />
      )}
    </div>
  );
}

/**
 * Library → Artists: an overview of every artist in the library with a
 * filter beside the open artist's albums, so switching artists never
 * leaves the page.
 */
export function LibraryArtistsPage() {
  const { t } = useLingui();
  const index = useArtistIndex();
  const gate = useLibraryGate({
    total: index.data?.length ?? 0,
    pending: index.isPending,
    error: index.error,
    retry: () => void index.refetch(),
    retrying: index.isRefetching,
    kind: "artists",
    empty: t`Every artist with a song or album in your library is listed here, A to Z.`,
    frame: "box",
  });
  if (gate) {
    return (
      <>
        <PageHeader title={t`Artists`} />
        {gate}
      </>
    );
  }
  return <ArtistsOverview artists={index.data} />;
}
