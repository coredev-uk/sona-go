import { PlaylistsService, Source, type PlaylistDetail, type Song } from "@bindings/services";
import { t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQueryClient } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { Check, Pencil, Plus } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { toast } from "sonner";

import { playSongs, shuffleSongs } from "@/features/player";
import { ActionMenu } from "@/shared/actions/ActionMenu";
import { Artwork } from "@/shared/components/Artwork";
import { CollectionHeader } from "@/shared/components/CollectionHeader";
import { CoverRow } from "@/shared/components/CoverRow";
import { DetailError, DetailSkeleton } from "@/shared/components/DetailStates";
import { EmptyState } from "@/shared/components/EmptyState";
import { LibraryButton } from "@/shared/components/LibraryButton";
import { PlayButtons } from "@/shared/components/PlayButtons";
import { RatingButtons } from "@/shared/components/RatingButtons";
import { Shelf } from "@/shared/components/Shelf";
import { callFailure } from "@/shared/lib/errors";
import { catalogueLine, formatTime, relativeTime } from "@/shared/lib/format";
import { TrackList } from "@/shared/tracks/TrackList";
import { Button } from "@/shared/ui/button";

import { showDialog } from "./dialogs";
import { serial } from "./edit";
import { moveTrack, removeTracks } from "./track-edits";
import { PlaylistEditor } from "./PlaylistEditor";
import { playlistKeys, useSuggestions, usePlaylist } from "./queries";

const route = getRouteApi("/playlist/$id");
const LINK = "hover:underline underline-offset-2";

/** Who made the playlist: a link to the curator's or profile's page when Sona has one. */
function Byline({ d }: { d: PlaylistDetail }) {
  const { curator } = d;
  const name = curator.name || d.playlist.curatorName;
  if (!name) return null;
  const link = LINK;
  if (curator.id && (curator.kind === "apple-curator" || curator.kind === "curator")) {
    return (
      <Link to="/curator/$id" params={{ id: curator.id }} className={link}>
        {name}
      </Link>
    );
  }
  if (curator.id && curator.kind === "profile") {
    return (
      <Link to="/profile/$id" params={{ id: curator.id }} className={link}>
        {name}
      </Link>
    );
  }
  return <span className="text-ink-muted">{name}</span>;
}

function metaLine(d: PlaylistDetail): string {
  const kind =
    d.kind === "personal-mix"
      ? t`Made for you`
      : d.isChart
        ? t`Chart`
        : d.favorited
          ? t`Your favourites`
          : t`Playlist`;
  const when = d.playlist.lastModified ? relativeTime(d.playlist.lastModified) : "";
  const updated = when ? t`Updated ${when}` : "";
  return [catalogueLine(kind, "", d.tracks.length, d.durationMs), updated]
    .filter(Boolean)
    .join(" · ");
}

export function PlaylistPage() {
  const { t } = useLingui();
  const { id } = route.useParams();
  const q = usePlaylist(id);
  const [editingTracks, setEditing] = useState(false);
  // Edit and Done replace each other: after a toggle focus moves to the
  // button that replaced the pressed one instead of falling to the page.
  const [toggled, setToggled] = useState(false);
  const toggleEditing = (on: boolean) => {
    setToggled(true);
    setEditing(on);
  };
  const qc = useQueryClient();
  // Row menus on an editable playlist offer "Remove from this playlist".
  const container = useMemo(() => ({ kind: "playlist", id, editable: true }), [id]);
  const tracks = q.data?.tracks;
  const onPlay = useCallback(
    (i: number) => tracks && playSongs(tracks, i, { kind: "playlist", id }),
    [tracks, id],
  );
  const editable = !!q.data && q.data.kind === "library" && q.data.playlist.canEdit;
  const suggestions = useSuggestions(id, editable && !editingTracks);

  if (q.isPending) return <DetailSkeleton />;
  if (q.error) {
    return (
      <DetailError
        noun="playlist"
        error={q.error}
        retry={() => void q.refetch()}
        retrying={q.isRefetching}
        back={{ label: t`Go to Playlists`, to: "/library/playlists" }}
      />
    );
  }

  const d = q.data;
  const { playlist } = d;
  const list = d.tracks;
  const inEdit = editable && editingTracks;
  const curatorName = d.curator.name;
  const playlistName = playlist.name;

  const move = (from: number, to: number) => moveTrack(qc, id, from, to, list[from]);
  const remove = (i: number) => removeTracks(qc, id, [i], [list[i]]);

  return (
    <>
      <CollectionHeader
        title={playlist.name}
        art={playlist.art.large}
        header={d.header}
        byline={<Byline d={d} />}
        meta={metaLine(d)}
        description={playlist.description}
        actions={
          inEdit ? (
            <>
              <Button size="sm" autoFocus={toggled} onClick={() => toggleEditing(false)}>
                <Check strokeWidth={1.75} />
                <Trans>Done</Trans>
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  showDialog({
                    kind: "details",
                    id,
                    name: playlist.name,
                    description: playlist.description,
                    isPublic: d.isPublic,
                  })
                }
              >
                <Trans>Name and description</Trans>
              </Button>
            </>
          ) : (
            <>
              <PlayButtons
                label={playlist.name}
                disabled={!list.some((t) => t.playable)}
                onPlay={() => playSongs(list, 0, { kind: "playlist", id })}
                onShuffle={() => void shuffleSongs(list, { kind: "playlist", id })}
              />
              {playlist.source === Source.SourceCatalog && (
                <LibraryButton kind="playlists" item={playlist} name={playlist.name} />
              )}
              <RatingButtons kind="playlists" item={playlist} name={playlist.name} />
              {editable && (
                <Button
                  variant="outline"
                  size="sm"
                  autoFocus={toggled}
                  onClick={() => toggleEditing(true)}
                >
                  <Pencil strokeWidth={1.75} />
                  <Trans>Edit</Trans>
                </Button>
              )}
              <ActionMenu kind="playlist" target={playlist} name={playlist.name} variant="header" />
            </>
          )
        }
      />
      {inEdit ? (
        list.length ? (
          <PlaylistEditor tracks={list} name={playlist.name} onMove={move} onRemove={remove} />
        ) : (
          <EmptyState title={t`Nothing left to arrange`}>
            <Trans>Add songs with “Add to playlist” in any song, album or playlist menu.</Trans>
          </EmptyState>
        )
      ) : list.length ? (
        <TrackList
          songs={list}
          columns={{ number: "row", artwork: true, artist: true, album: true }}
          label={t`Songs in ${playlistName}`}
          onPlay={onPlay}
          container={editable ? container : undefined}
        />
      ) : (
        <EmptyState title={t`This playlist is empty`}>
          {editable
            ? t`Add songs with “Add to playlist” in any song, album or playlist menu, or pick from the suggestions below.`
            : t`Songs added to it in Apple Music on any device appear here after the next sync.`}
        </EmptyState>
      )}
      {editable && !inEdit && (
        <Suggestions
          songs={suggestions.data}
          loading={suggestions.isPending}
          failed={!!suggestions.error}
          playlistId={id}
          playlistName={playlist.name}
        />
      )}
      {d.featuredArtists.length > 0 && (
        <Shelf title={t`Featured artists`} className="mt-12">
          <CoverRow
            label={t`Featured artists`}
            items={d.featuredArtists.map((a) => ({
              key: a.id,
              title: a.name,
              art: a.art.medium,
              motion: a.motion || undefined,
              shape: "round" as const,
              link: { to: "/artist/$id", params: { id: a.id } },
            }))}
          />
        </Shelf>
      )}
      {d.moreByCurator.length > 0 && (
        <Shelf
          title={curatorName ? t`More from ${curatorName}` : t`More like this`}
          className="mt-12"
        >
          <CoverRow
            label={t`More playlists from this curator`}
            items={d.moreByCurator.map((p) => ({
              key: p.id,
              title: p.name,
              subtitle: p.curatorName,
              art: p.art.medium,
              motion: p.motion || undefined,
              link: { to: "/playlist/$id", params: { id: p.id } },
            }))}
          />
        </Shelf>
      )}
    </>
  );
}

/**
 * Songs Apple suggests for a library playlist: each plays on click and
 * joins the playlist with its add button.
 */
function Suggestions({
  songs,
  loading,
  failed,
  playlistId,
  playlistName,
}: {
  songs: Song[] | undefined;
  loading: boolean;
  failed: boolean;
  playlistId: string;
  playlistName: string;
}) {
  const { t } = useLingui();
  const qc = useQueryClient();
  const [added, setAdded] = useState<ReadonlySet<string>>(new Set());
  if (failed || (!loading && !songs?.length)) return null;
  const add = (s: Song) => {
    setAdded((prev) => new Set(prev).add(s.id));
    void serial(playlistId, () =>
      PlaylistsService.Add(playlistId, [{ kind: "song", id: s.id, source: s.source }]),
    ).then(
      (d) => qc.setQueryData(playlistKeys.detail(playlistId), d),
      (err) => {
        setAdded((prev) => {
          const next = new Set(prev);
          next.delete(s.id);
          return next;
        });
        const songName = s.name;
        toast.error(t`Couldn't add “${songName}” to “${playlistName}”`, {
          description: callFailure(err).message,
        });
      },
    );
  };
  return (
    <Shelf title={t`Suggested songs`} className="mt-12">
      <ul className="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-x-6">
        {(songs ?? Array.from({ length: 6 }, () => undefined)).map((s, i) =>
          s ? (
            <li
              key={s.id}
              className="group/row flex h-14 items-center gap-3 rounded-md px-2 hover:bg-hover"
            >
              <button
                type="button"
                aria-label={t`Play ${{ songName: s.name }}`}
                onClick={() => playSongs([s], 0, playlistId)}
                className="shrink-0 rounded-sm"
              >
                <Artwork src={s.art.small} size={40} alt="" />
              </button>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-ui text-ink">{s.name}</span>
                <span className="block truncate text-sub text-ink-muted">
                  {s.artistName}
                  {s.durationMs ? <span className="num"> · {formatTime(s.durationMs)}</span> : null}
                </span>
              </span>
              {added.has(s.id) ? (
                <span className="flex h-8 items-center gap-1 px-2 text-sub text-ink-muted">
                  <Check aria-hidden className="size-4" strokeWidth={1.75} />
                  <Trans>Added</Trans>
                </span>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t`Add ${{ songName: s.name }} to ${playlistName}`}
                  onClick={() => add(s)}
                  className="text-ink-muted hover:text-ink"
                >
                  <Plus strokeWidth={1.75} />
                  <Trans>Add</Trans>
                </Button>
              )}
            </li>
          ) : (
            <li key={i} aria-hidden className="flex h-14 items-center gap-3 px-2">
              <span className="size-10 rounded-sm bg-hover" />
              <span className="flex-1">
                <span className="block h-3.5 w-1/2 rounded-sm bg-hover" />
                <span className="mt-1.5 block h-3 w-1/3 rounded-sm bg-hover" />
              </span>
            </li>
          ),
        )}
      </ul>
    </Shelf>
  );
}
