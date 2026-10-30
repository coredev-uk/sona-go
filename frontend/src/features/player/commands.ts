import { PlaybackService, type Song, type TrackRef } from "@bindings/services";
import { toast } from "sonner";

import { run } from "@/shared/lib/run";

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

/** Plays `songs` from `start` as the queue, labelled with the collection it came from. */
export function playSongs(songs: readonly (Song | undefined)[], start: number, label: string) {
  const { refs, index } = playable(songs, start);
  if (!refs.length) {
    toast.error("Nothing here can be played", {
      description: "None of these songs is available to stream.",
    });
    return;
  }
  void run("Couldn't start playback", PlaybackService.PlayItems(refs, index, label));
}

/** Shuffle-plays `songs`: shuffle on, starting from a random playable song. */
export async function shuffleSongs(songs: readonly (Song | undefined)[], label: string) {
  const { refs } = playable(songs, 0);
  if (!refs.length) return;
  if (!(await run("Couldn't turn on shuffle", PlaybackService.SetShuffle(true)))) return;
  const start = Math.floor(Math.random() * refs.length);
  void run("Couldn't start playback", PlaybackService.PlayItems(refs, start, label));
}

export function playNext(songs: Song[]) {
  void run("Couldn't add to the queue", PlaybackService.PlayNext(playable(songs, 0).refs)).then(
    (ok) => ok && toast(songs.length === 1 ? `“${songs[0].name}” plays next` : "Plays next"),
  );
}

export function addToQueue(songs: Song[]) {
  void run("Couldn't add to the queue", PlaybackService.Append(playable(songs, 0).refs)).then(
    (ok) =>
      ok &&
      toast(songs.length === 1 ? `Added “${songs[0].name}” to the queue` : "Added to the queue"),
  );
}
