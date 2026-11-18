---
title: Command Line
description: Control the running Sona from a terminal, a script or a status bar.
---

`sona <command>` controls the Sona that is running: playback, the queue, the volume, favourites, your library, lyrics, search, links, private listening and Spatial Audio. It prints text for people and, with `--json`, one line of JSON for scripts, status bars and stream decks. Commands never start Sona.

```sh
sona status                 # what's playing
sona toggle                 # play/pause
sona next; sona prev
sona seek +15               # seconds into the song, or +N/-N
sona volume +10             # 0 to 100, or +N/-N
sona shuffle toggle; sona repeat cycle; sona crossfade on
sona like                   # favourite the playing song (dislike: Suggest Less)
sona library add            # add the playing song to your library
sona library playlists      # your playlists, with links to play
sona play --shuffle https://music.apple.com/library/playlist/p.…
sona queue jump 3           # play the third song in the queue
sona lyrics                 # the playing song's lyrics (--lrc for an LRC file)
sona search abbey road      # results with their links
sona open https://music.apple.com/gb/album/abbey-road/1441164426
sona private on
sona help [command]         # sona(1) has the same
```

Started without a command, `sona` opens Sona, or hands its Apple Music links to the Sona already running. `sona --hidden` starts without a window and `sona --version` prints the version.

`sona status --follow` and `sona lyrics --follow` keep printing as things change, which suits Waybar, Polybar and similar bars. `sona connect` keeps a two-way line open for a program of yours, such as [Sona for Stream Deck](https://github.com/coredev-uk/sona-streamdeck). Shell completions for bash, zsh and fish come from `sona completion`.

Every command, option, exit status and JSON schema is in the [command line reference](/reference/cli/).
