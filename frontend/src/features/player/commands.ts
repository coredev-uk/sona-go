import {
  PlaybackService,
  type MusicVideo,
  type PlayFrom,
  type Song,
  type TrackRef,
} from "@bindings/services";
import { plural, t } from "@lingui/core/macro";
import { toast } from "sonner";

import { describeItems, type ActionItem } from "@/shared/actions/registry";
import { tracksOf } from "@/shared/actions/songs";
import { callFailure } from "@/shared/lib/errors";
import { fetchItemState } from "@/shared/lib/item-state";
import { rateItem } from "@/shared/lib/library-actions";
import { run } from "@/shared/lib/run";

import { refuse } from "./availability";
import { livePosition, usePlayback, type Repeat } from "./playback";
import { replaceQueue } from "./replace";
import { useUI } from "./ui";

/** Songs that can be queued, and where `start` lands among them. */
export function playable(songs: readonly (Song | undefined)[], start: number) {
  const refs: TrackRef[] = [];
  let index = 0;
  songs.forEach((s, i) => {
    if (!s?.playable) return;
    if (i === start) index = refs.length;
    refs.push({ id: s.id, source: s.source });
  });
  return { refs, index };
}

/**
 * Where songs come from: an Apple album, playlist or artist (reported with
 * the plays), or a label for anything else ("search", a chart's title).
 */
export type PlaySource = PlayFrom | string;

/** Plays `songs` from `start` as the queue, from the collection `from`. */
export function playSongs(songs: readonly (Song | undefined)[], start: number, from: PlaySource) {
  const { refs, index } = playable(songs, start);
  if (!refs.length) {
    toast.error(t`Nothing here can be played`, {
      description: t`None of these songs is available to stream.`,
    });
    return;
  }
  const playFrom = typeof from === "string" ? { kind: "", id: from } : from;
  void replaceQueue(() =>
    run(t`Couldn't start playback`, PlaybackService.PlayItems(refs, index, playFrom)),
  );
}

/** Shuffle-plays `songs`: shuffle on, starting from a random playable song. */
export async function shuffleSongs(songs: readonly (Song | undefined)[], from: PlaySource) {
  const { refs } = playable(songs, 0);
  if (!refs.length) return;
  if (!(await run(t`Couldn't turn on shuffle`, PlaybackService.SetShuffle(true)))) return;
  const start = Math.floor(Math.random() * refs.length);
  const playFrom = typeof from === "string" ? { kind: "", id: from } : from;
  void replaceQueue(() =>
    run(t`Couldn't start playback`, PlaybackService.PlayItems(refs, start, playFrom)),
  );
}

/** A music video's queue entry: TrackRef.kind "music-video" (Go's services.TrackKindVideo; "" is a song). */
function videoRef(v: MusicVideo): TrackRef {
  return { id: v.id, source: v.source, kind: "music-video" };
}

/**
 * Plays `videos` from `start` as the queue (a library copy plays its
 * catalogue video) and opens the video view. Videos are never reported as
 * plays, so they carry no PlayFrom collection.
 */
export async function playVideos(videos: readonly MusicVideo[], start: number) {
  if (!videos.length) return;
  const refs = videos.map(videoRef);
  const started = await replaceQueue(() =>
    run(t`Couldn't start playback`, PlaybackService.PlayItems(refs, start, { kind: "", id: "" })),
  );
  if (started) useUI.getState().openVideo();
}

/**
 * Sets the sleep timer: mode "after" (in `minutes`), "track" or "queue";
 * "" turns it off.
 */
export function setSleep(mode: "" | "after" | "track" | "queue", minutes = 0) {
  void run(t`Couldn't set the sleep timer`, PlaybackService.SetSleep(mode, minutes));
}

/**
 * Queues songs, music videos, albums and playlists (their tracks), in
 * order: `"next"` after the current item, `"last"` after the user's
 * queue, or at queue position `at` (a drop on the queue panel).
 */
export async function queueItems(items: readonly ActionItem[], where: "next" | "last" | number) {
  if (refuse("queue")) return;
  let refs: TrackRef[];
  try {
    const lists = await Promise.all(
      items.map(async (i) =>
        i.kind === "music-video" ? [videoRef(i.target)] : playable(await tracksOf(i), 0).refs,
      ),
    );
    refs = lists.flat();
  } catch (err) {
    toast.error(t`Couldn't add to the queue`, { description: callFailure(err).message });
    return;
  }
  if (!refs.length) {
    toast.error(t`Nothing here can be played`, {
      description: t`None of these songs is available to stream.`,
    });
    return;
  }
  const call =
    where === "next"
      ? PlaybackService.PlayNext(refs)
      : where === "last"
        ? PlaybackService.Append(refs)
        : PlaybackService.Insert(where, refs);
  if (!(await run(t`Couldn't add to the queue`, call))) return;
  const what = describeItems(items);
  const count = items.length;
  toast(
    where === "next"
      ? plural(count, { one: `${what} plays next`, other: `${what} play next` })
      : t`Added ${what} to the queue`,
  );
}

/** Seeks `deltaMs` from the live position, within the track (a live station says it can't). */
export function seekBy(deltaMs: number) {
  const s = usePlayback.getState();
  if (!s.current || refuse("seek") || s.durationMs <= 0) return;
  const to = Math.min(Math.max(livePosition(s, Date.now()) + deltaMs, 0), s.durationMs);
  s.patch({ positionMs: to, at: Date.now() });
  void run(t`Couldn't seek`, PlaybackService.Seek(to));
}

export function setVolume(v: number) {
  const volume = Math.min(Math.max(v, 0), 1);
  usePlayback.getState().patch({ volume });
  void run(t`Couldn't change the volume`, PlaybackService.SetVolume(volume));
}

/** The volume to return to on unmute (the bar's button and the M key share it). */
let unmuted = 0.8;

export function toggleMute() {
  const volume = usePlayback.getState().volume;
  if (volume > 0) unmuted = volume;
  setVolume(volume > 0 ? 0 : unmuted || 0.8);
}

export function toggleShuffle() {
  const s = usePlayback.getState();
  if (!s.current || refuse("shuffle")) return;
  void run(t`Couldn't change shuffle`, PlaybackService.SetShuffle(!s.shuffle));
}

export const nextRepeat: Record<Repeat, Repeat> = { off: "all", all: "one", one: "off" };

export function cycleRepeat() {
  const s = usePlayback.getState();
  if (!s.current || refuse("repeat")) return;
  void run(t`Couldn't change repeat`, PlaybackService.SetRepeat(nextRepeat[s.repeat as Repeat]));
}

/** Favourites the playing song, or undoes it (not live radio, music videos or songs outside Apple Music). */
export async function toggleFavourite() {
  const current = usePlayback.getState().current;
  if (!current || current.live || current.video || !(current.catalogId || current.libraryId)) {
    return;
  }
  const state = await fetchItemState("songs", current);
  rateItem("songs", current, current.name, state?.rating === 1 ? "unfavorite" : "favorite");
}
