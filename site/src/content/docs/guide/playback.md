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
- Where the singing stops for five seconds or more, three dots stand in the lyrics and fill in amber, one after the other, until the next line begins.
- Press [ and ] to shift the lyrics earlier or later, and \ to reset.
- Choose the sources and a translation language in Settings → Playback → Lyrics.

## Full-Screen Lyrics

Press Shift+L, or choose Full screen under the lyrics in Now Playing, to give the whole screen to the words. The window goes full screen. Every line is set in the same large size. The sung line sits in the middle, with the lines before it fading above and the next two below, and the words light in amber as they are sung. In a long instrumental gap three dots take the middle and fill in amber until the singing returns. It suits a TV or a second screen across a room.

- The song, previous, play or pause and next, its progress, the lyrics timing and Exit full screen are hidden until you move the pointer or press a key, and hide again after three seconds. Music that is paused keeps them in view.
- Press Space to play or pause, ↑ and ↓ to go to the previous or next line, ← and → to go back or ahead 5 seconds, and Home for the start of the song. Escape or Shift+L leaves.
- The screen stays on while the music plays, so a TV left on doesn't go dark. The view is dark in a light theme too.
- A song with no lyrics shows its cover large, with the title and the artist.
- With reduce motion on, lines jump to their place instead of climbing and the background holds still. The background also holds still on battery.

## Open Now Playing

Press N, or select the cover in the player bar, for a full view with the lyrics and Up next. Press Escape to close it. V opens the music video when a song has one.

## Use the mini player

Press Ctrl+Shift+M, or use the mini player button, for a small window that stays on top. On Wayland some compositors ignore always-on-top.

## Tune the sound

- **Equaliser**: press Ctrl+Shift+E for ten bands, presets and a preamp. You hear changes as you move them.
- **Crossfade** and **Sound Check** (every song at about the same loudness): Settings → Playback → Sound.
- **Spatial Audio**: Settings → Playback → Sound. It is for headphones: it plays Apple's own headphone mix of songs made in Dolby Atmos. It works for many of them but not all, because Apple offers some only to its own apps; those play in stereo. It applies at once, so you can turn it on and off while a song plays and hear the two mixes at the same place in the song. A quiet **Spatial** badge beside the title in the player bar (and under the title in Now Playing) shows when the song playing is the Spatial Audio one. When it is on and a song that has a Spatial Audio mix plays in stereo, Now Playing says why under the album, such as "Stereo · Apple keeps this song's Spatial Audio mix for its own apps".
- **Output device**: choose it in Settings → Playback. Switching moves the music without a pause. If the device is unplugged, Sona plays to the system default until it returns.

## Set volume

Drag the Volume slider, scroll the mouse wheel or swipe over it, or press Ctrl+↑ and Ctrl+↓. M mutes.

## Set a sleep timer

Choose Sleep timer in the player bar, then pick 15, 30 or 45 minutes, 1 or 2 hours, the end of this song, or the end of the queue. Sona fades out over the last 10 seconds and pauses.
