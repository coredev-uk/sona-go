import { PlaybackService } from "@bindings/services";
import { t } from "@lingui/core/macro";
import {
  Clapperboard,
  Disc3,
  FastForward,
  ListMusic,
  Maximize2,
  MicVocal,
  Minus,
  Pause,
  Plus,
  Repeat,
  Rewind,
  Shuffle,
  SkipBack,
  SkipForward,
  Moon,
  Star,
  TimerReset,
  Volume1,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";

import { currentCapabilities } from "@/shared/lib/capabilities";
import { run } from "@/shared/lib/run";
import type { Shortcut } from "@/shared/module";

import {
  cycleRepeat,
  seekBy,
  setSleep,
  setVolume,
  toggleFavourite,
  toggleMute,
  toggleShuffle,
} from "./commands";
import { useLyricsOffset } from "./lyrics";
import { lyricsSongId, usePlayback } from "./playback";
import { useUI } from "./ui";

const SEEK_MS = 10_000;
const VOLUME_STEP = 0.05;

/** The playing song's lyrics are on screen (now playing with lyrics, or full screen). */
function lyricsShown(): boolean {
  const ui = useUI.getState();
  const shown = ui.overlay === "room" || (ui.overlay === "now-playing" && ui.showLyrics);
  return shown && !!lyricsSongId(usePlayback.getState().current);
}

function nudgeLyrics(dir: 1 | -1) {
  const id = lyricsSongId(usePlayback.getState().current);
  if (id) useLyricsOffset.getState().nudge(id, dir);
}

/** A music video is playing: no lyrics, no song to favourite. */
function videoPlaying(): boolean {
  return !!usePlayback.getState().current?.video;
}

/** Sona sets the playback volume on this platform (volume keys and mute). */
function volumeSupported(): boolean {
  return currentCapabilities()?.playback.volume ?? false;
}

/**
 * Transport and the player's views. Plain letters work anywhere outside a
 * text field; Space also leaves buttons and sliders alone. Escape closes
 * the now-playing view while it is open.
 */
export const playerShortcuts: Shortcut[] = [
  {
    id: "player.toggle",
    label: () => t`Play or pause`,
    group: "playback",
    keys: ["Space"],
    keywords: () => t`resume stop`,
    icon: Pause,
    run: () => void run(t`Couldn't play or pause`, PlaybackService.Toggle()),
  },
  {
    id: "player.next",
    label: () => t`Next track`,
    group: "playback",
    keys: ["Ctrl+ArrowRight"],
    keywords: () => t`skip forward`,
    icon: SkipForward,
    run: () => void run(t`Couldn't skip`, PlaybackService.Next()),
  },
  {
    id: "player.previous",
    label: () => t`Previous track`,
    group: "playback",
    keys: ["Ctrl+ArrowLeft"],
    keywords: () => t`back restart`,
    icon: SkipBack,
    run: () => void run(t`Couldn't go back`, PlaybackService.Prev()),
  },
  {
    id: "player.seek-forward",
    label: () => t`Skip ahead 10 seconds`,
    group: "playback",
    keys: ["Shift+ArrowRight"],
    keywords: () => t`seek forward`,
    icon: FastForward,
    repeat: true,
    run: () => seekBy(SEEK_MS),
  },
  {
    id: "player.seek-back",
    label: () => t`Go back 10 seconds`,
    group: "playback",
    keys: ["Shift+ArrowLeft"],
    keywords: () => t`seek rewind`,
    icon: Rewind,
    repeat: true,
    run: () => seekBy(-SEEK_MS),
  },
  {
    id: "player.volume-up",
    label: () => t`Volume up`,
    group: "playback",
    keys: ["Ctrl+ArrowUp"],
    keywords: () => t`louder`,
    icon: Volume2,
    repeat: true,
    when: volumeSupported,
    run: () => setVolume(usePlayback.getState().volume + VOLUME_STEP),
  },
  {
    id: "player.volume-down",
    label: () => t`Volume down`,
    group: "playback",
    keys: ["Ctrl+ArrowDown"],
    keywords: () => t`quieter`,
    icon: Volume1,
    repeat: true,
    when: volumeSupported,
    run: () => setVolume(usePlayback.getState().volume - VOLUME_STEP),
  },
  {
    id: "player.mute",
    label: () => t`Mute or unmute`,
    group: "playback",
    keys: ["M"],
    keywords: () => t`silence sound`,
    icon: VolumeX,
    when: volumeSupported,
    run: toggleMute,
  },
  {
    id: "player.shuffle",
    label: () => t`Shuffle on or off`,
    group: "playback",
    keys: ["S"],
    keywords: () => t`random`,
    icon: Shuffle,
    run: toggleShuffle,
  },
  {
    id: "player.repeat",
    label: () => t`Repeat off, all or one`,
    group: "playback",
    keys: ["R"],
    keywords: () => t`loop`,
    icon: Repeat,
    run: cycleRepeat,
  },
  {
    id: "player.favourite",
    label: () => t`Favourite the playing song`,
    group: "playback",
    keys: ["F"],
    keywords: () => t`like love star favorite`,
    icon: Star,
    when: () => !videoPlaying(),
    run: () => void toggleFavourite(),
  },
  {
    id: "player.queue",
    label: () => t`Show or hide the queue`,
    group: "view",
    keys: ["Q"],
    keywords: () => t`up next`,
    icon: ListMusic,
    run: () => useUI.getState().toggleQueue(),
  },
  {
    id: "player.lyrics",
    label: () => t`Show or hide lyrics`,
    group: "view",
    keys: ["L"],
    keywords: () => t`words sing`,
    icon: MicVocal,
    when: () => !videoPlaying(),
    run: () => {
      const ui = useUI.getState();
      if (ui.overlay === "now-playing" && ui.showLyrics) ui.closeOverlay();
      else if (usePlayback.getState().current) ui.openNowPlaying(true);
    },
  },
  {
    id: "player.now-playing",
    label: () => t`Show or hide now playing`,
    group: "view",
    keys: ["N"],
    keywords: () => t`cover artwork full`,
    icon: Disc3,
    run: () => {
      const ui = useUI.getState();
      if (ui.overlay === "now-playing" && !ui.showLyrics) ui.closeOverlay();
      else if (usePlayback.getState().current) ui.openNowPlaying(false);
    },
  },
  {
    id: "player.video",
    label: () => t`Show or hide the video`,
    group: "view",
    keys: ["V"],
    keywords: () => t`music video picture watch`,
    icon: Clapperboard,
    when: videoPlaying,
    run: () => {
      const ui = useUI.getState();
      if (ui.overlay === "video") ui.closeOverlay();
      else ui.openVideo();
    },
  },
  {
    id: "player.close-overlay",
    label: () => t`Close now playing`,
    group: "view",
    keys: ["Escape"],
    scope: "now-playing",
    icon: X,
    when: () => useUI.getState().overlay !== "none",
    palette: false,
    run: () => useUI.getState().closeOverlay(),
  },
  {
    id: "player.lyrics-earlier",
    label: () => t`Show lyrics half a second earlier`,
    group: "view",
    keys: ["["],
    keywords: () => t`lyrics timing offset sync delay`,
    icon: Minus,
    repeat: true,
    when: lyricsShown,
    run: () => nudgeLyrics(-1),
  },
  {
    id: "player.lyrics-later",
    label: () => t`Show lyrics half a second later`,
    group: "view",
    keys: ["]"],
    keywords: () => t`lyrics timing offset sync delay`,
    icon: Plus,
    repeat: true,
    when: lyricsShown,
    run: () => nudgeLyrics(1),
  },
  {
    id: "player.lyrics-reset",
    label: () => t`Put the lyrics back on their original timing`,
    group: "view",
    keys: ["\\"],
    keywords: () => t`lyrics timing offset sync reset`,
    icon: TimerReset,
    when: lyricsShown,
    run: () => {
      const id = lyricsSongId(usePlayback.getState().current);
      if (id) useLyricsOffset.getState().reset(id);
    },
  },
  {
    id: "player.lyrics-room",
    label: () => t`Full-screen lyrics`,
    group: "view",
    keys: ["Shift+L"],
    keywords: () => t`lyrics room karaoke big large tv`,
    icon: Maximize2,
    when: () => !!lyricsSongId(usePlayback.getState().current),
    run: () => {
      const ui = useUI.getState();
      if (ui.overlay === "room") ui.closeOverlay();
      else ui.openRoom();
    },
  },
  // The sleep timer, from the command palette (no default keys).
  ...(
    [
      ["player.sleep-30", () => t`Sleep timer: stop in 30 minutes`, "after", 30],
      ["player.sleep-60", () => t`Sleep timer: stop in 1 hour`, "after", 60],
      ["player.sleep-track", () => t`Sleep timer: stop after this song`, "track", 0],
      ["player.sleep-queue", () => t`Sleep timer: stop after the queue`, "queue", 0],
    ] as const
  ).map(([id, label, mode, minutes]): Shortcut => ({
    id,
    label,
    group: "playback",
    keys: [],
    keywords: () => t`sleep timer bedtime fade`,
    icon: Moon,
    when: () => !!usePlayback.getState().current,
    run: () => setSleep(mode, minutes),
  })),
  {
    id: "player.sleep-off",
    label: () => t`Turn off the sleep timer`,
    group: "playback",
    keys: [],
    keywords: () => t`sleep timer cancel`,
    icon: Moon,
    when: () => !!usePlayback.getState().sleep,
    run: () => setSleep(""),
  },
];
