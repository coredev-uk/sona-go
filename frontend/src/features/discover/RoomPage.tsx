import { DiscoverService, type Item, type RoomPage as Room, type Song } from "@bindings/services";
import { getRouteApi } from "@tanstack/react-router";
import { useCallback, useMemo, type ReactNode } from "react";

import { playSongs, shuffleSongs } from "@/features/player";
import { PageHeader } from "@/shared/components/PageHeader";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { ItemGrid } from "@/shared/items/ItemGrid";
import { TrackList } from "@/shared/tracks/TrackList";

import { Choice } from "./Choice";
import { discoverKeys, useMorePages, useRoom } from "./queries";
import { PageError, ShelvesSkeleton } from "./states";

const route = getRouteApi("/room/$id");

const sortLabels: Record<string, string> = {
  featured: "Editors' order",
  alphabet: "A to Z",
  releaseDate: "Newest first",
};

/** An editorial section in full ("See all"), in pages of 100. */
export function RoomRoutePage() {
  const { id } = route.useParams();
  const { sort = "" } = route.useSearch();
  const navigate = route.useNavigate();
  const q = useRoom(id, sort);
  if (q.isPending) {
    return (
      <>
        <PageHeader title="" />
        <ShelvesSkeleton />
      </>
    );
  }
  if (q.isError) {
    return (
      <PageError
        what="this list"
        error={q.error}
        onRetry={() => void q.refetch()}
        retrying={q.isRefetching}
      />
    );
  }
  const room = q.data;
  const sorts =
    room.sorts.length > 1 ? (
      <Choice
        label="Order"
        value={room.sort}
        options={room.sorts.map((s) => ({ value: s, label: sortLabels[s] ?? s }))}
        onChange={(v) => void navigate({ search: { sort: v || undefined }, replace: true })}
      />
    ) : undefined;
  return <RoomContent key={`${id}|${sort}`} room={room} sort={sort} sorts={sorts} />;
}

function RoomContent({ room, sort, sorts }: { room: Room; sort: string; sorts?: ReactNode }) {
  const more = useMorePages(discoverKeys.room(room.id, sort), room.next, (offset) =>
    DiscoverService.RoomItems(room.id, sort, offset),
  );
  const items: Item[] = useMemo(
    () => [...room.items, ...(more.data?.pages.flatMap((p) => p.items) ?? [])],
    [room.items, more.data],
  );
  const hasMore = room.next > 0 && (!more.data || more.hasNextPage);
  const { fetchNextPage, isFetching } = more;
  const onRange = useCallback(
    (_: number, end: number) => {
      if (hasMore && !isFetching && end >= items.length - 24) void fetchNextPage();
    },
    [hasMore, isFetching, items.length, fetchNextPage],
  );

  if (room.kind === "tracks") {
    const songs: Song[] = items.flatMap((i) => (i.song ? [i.song] : []));
    return (
      <>
        <PageHeader
          title={room.title}
          actions={
            <>
              <PlayButtons
                label={room.title}
                disabled={!songs.some((s) => s.playable)}
                onPlay={() => playSongs(songs, 0, room.title)}
                onShuffle={() => void shuffleSongs(songs, room.title)}
              />
              {sorts}
            </>
          }
        />
        <TrackList
          songs={hasMore ? [...songs, undefined] : songs}
          columns={{ number: "row", artwork: true, artist: true, album: true }}
          label={room.title}
          onPlay={(i) => playSongs(songs, i, room.title)}
          onRange={onRange}
        />
      </>
    );
  }
  return (
    <>
      <PageHeader title={room.title} actions={sorts} />
      <ItemGrid
        items={hasMore ? [...items, ...Array<undefined>(12)] : items}
        label={room.title}
        onRange={onRange}
      />
    </>
  );
}
