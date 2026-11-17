---
title: Playback
description: Control the queue, shuffle, repeat, lyrics, the equaliser, the sleep timer and the output.
---

The player bar along the bottom controls what plays. Every control here also has a [keyboard shortcut](/guide/keyboard-shortcuts/) and works from the [command line](/guide/cli/).

## Use the queue

Open the queue with the Queue button in the player bar, or press Q.

- Play next and Play last in any item's menu add songs to the queue.
- Drag rows to reorder them, or press Alt+↑ and Alt+↓ on a focused row.
- Remove takes a song out. Clear empties the queue.
- AutoPlay adds similar songs when your own songs run out.

Sona keeps your queue between sessions. When you play an album or playlist and more than three songs you added are waiting, Sona asks first. You can change that in Settings → Playback.

## Shuffle and repeat

Shuffle (S) plays the queue in a random order. Repeat (R) cycles through repeat off, repeat all and repeat one.

## Read lyrics

Press L to open the lyrics in Now Playing. Sona asks every lyrics source you turned on and shows the finest timing it finds: word by word, then line by line, then plain text.

- Select a line to jump to it.
- Press [ and ] to shift the lyrics earlier or later, and \ to reset.
- Choose the sources and a translation language in Settings → Playback → Lyrics.

## Open Now Playing

Press N, or select the cover in the player bar, for a full view with the lyrics and Up next. Press Escape to close it. V opens the music video when a song has one.

## Use the mini player

Press Ctrl+Shift+M, or use the mini player button, for a small window that stays on top. On Wayland some compositors ignore always-on-top.

## Tune the sound

- **Equaliser**: press Ctrl+Shift+E for ten bands, presets and a preamp. You hear changes as you move them.
- **Crossfade** and **Sound Check** (every song at about the same loudness): Settings → Playback → Sound.
- **Spatial Audio**: Settings → Playback → Sound. Songs with a Dolby Atmos mix play Apple's headphone rendition of it where Apple allows it for Sona, from the next song on; every other song plays in stereo. A quiet **Spatial** badge beside the title in the player bar (and under the title in Now Playing) shows when the song playing is the Spatial Audio one.
- **Output device**: choose it in Settings → Playback. Switching moves the music without a pause. If the device is unplugged, Sona plays to the system default until it returns.

## Set volume

Drag the Volume slider, scroll the mouse wheel or swipe over it, or press Ctrl+↑ and Ctrl+↓. M mutes.

## Set a sleep timer

Choose Sleep timer in the player bar, then pick 15, 30 or 45 minutes, 1 or 2 hours, the end of this song, or the end of the queue. Sona fades out over the last 10 seconds and pauses.
