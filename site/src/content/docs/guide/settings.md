---
title: Settings
description: Every section of Settings, what each setting does and which ones depend on your platform.
---

Open Settings from the sidebar or press Ctrl+,. Type in "Find a setting" above the section list to jump to a setting by name. Every change saves at once.

Some settings depend on your platform or build. Where a platform can't back a setting, Sona keeps its row and says why instead of hiding the reason. The tables below name those cases with _Linux_, _macOS_ and _Windows_.

## General

| Setting | What it does | Platform |
| --- | --- | --- |
| Language | Sets the language of Sona. Automatic follows your Apple Music account and your system. Shown only when other languages are on offer. | |
| Start Sona when you log in | Adds Sona to your desktop's startup applications. | Needs Sona.app on _macOS_, not a source build. On _Windows_ Task Manager's startup apps list it; a portable copy must stay in its folder, or be turned on again after a move. |
| Start in the tray | Starts Sona without a window at login, ready to play from the tray or your media keys. Shown when Start Sona when you log in is on. | Needs a tray icon |
| Open on | Chooses the page Sona shows when it starts: the page you were last on, or a fixed page. Your queue is always kept. | |
| Open Apple Music links in Sona | Makes Sona the app that opens `music://` and `itms://` links. Links that start with `https://` stay with your browser. | Needs Sona.app on _macOS_; needs Sona's desktop entry on _Linux_, so not a source build, Flatpak or AppImage that isn't integrated. Not on _Windows_ yet. |
| First-run setup | Runs "Set up Sona again": the steps of [first-run setup](/guide/first-run/). | |
| When the window is closed | Keeps playing in the background or quits Sona. In the background, Sona stays in the system tray. | Without a tray icon Sona quits. |
| Title bar | Chooses Automatic, Sona's title bar with or without window buttons, or the desktop's. Applies the next time Sona starts. | _Linux_ _Windows_ |
| Window size | Shows the size Sona opens at. Reset size puts it back to the default. | |
| Track notifications | Shows the song and artist, with artwork and a Next button, when a track starts. Not shown while Sona's window is in front. | _macOS_ and _Windows_ show the song and artist only, and a click opens Sona. _macOS_ needs Sona.app. _Linux_ needs a notification service. |
| Stay awake while playing | Stops the computer from suspending on its own while music plays. The screen can still lock and turn off. | Not on _Windows_ yet. On _Linux_ needs logind or the desktop portal. |

### Updates

| Setting | What it does | Platform |
| --- | --- | --- |
| Check for updates when Sona starts | Asks GitHub for the newest release once the window has opened. Nothing about you or your library is sent. | |
| Test versions | Also offers release candidates and other test versions. | Only where Sona can update itself |

This group also shows whether you're up to date, with View release, Install and restart and Check now. It doesn't check development builds. See [Install](/guide/install/) for how updating works.

## Playback

On a platform with no playback engine this section says so, and only Lyrics remains. Browsing Apple Music and your library works as usual.

### Sound

| Setting | What it does | Platform |
| --- | --- | --- |
| Widevine | Shows the state of the Widevine download that plays Apple Music's protected songs, with Try again if it failed. Sona keeps it up to date by itself. | |
| Audio quality | Chooses High (256 kbps AAC) or Data saver (64 kbps). Applies from the next song. | |
| Output | Chooses where Sona plays. Switching moves the music without a pause. | Where the engine can pick a device |
| Sound Check | Plays every song at about the same loudness. | Where the engine supports it |
| Equaliser | Opens the ten-band equaliser. | Where the engine supports it |
| Crossfade and Crossfade length | Fades each song into the next. Songs that follow each other on an album always play without a gap. | Where the engine supports it |

See [Playback](/guide/playback/) for the equaliser and the output in use.

### Listening

| Setting | What it does | Platform |
| --- | --- | --- |
| Private listening | Keeps plays out of your Apple Music history and recommendations, scrobbling, Discord and History. The same as the button in the player bar. See [Private listening](/guide/private-listening/). | Where Sona sends the play reports itself |
| Replacing your queue | Chooses "Replace, offer Undo" or "Ask first" when you play an album, playlist or station while more than three songs you added are waiting. | |

### Lyrics

| Setting | What it does |
| --- | --- |
| Lyrics sources | Turns Apple Music, Musixmatch, LRCLIB and NetEase Cloud Music on or off and orders them. A source higher in the list wins a tie. |
| Prefer word-by-word lyrics | Lets lyrics timed word by word win over line-by-line lyrics from a source higher in the list. |
| Lyrics translation | Shows a translation under each line where a source has one. |

### Advanced

| Setting | What it does |
| --- | --- |
| Free memory when idle | Stops the playback host after 5, 10, 15, 30 or 60 minutes with nothing queued. The next song then takes a few seconds longer to start. |

## Appearance

| Setting | What it does |
| --- | --- |
| Mode | Chooses System, Light or Dark. A theme with one variant keeps its look and disables this. |
| Theme | Chooses the colours of the whole window from Sona's own, community palettes and your themes. See [Themes](/guide/themes/). |
| Accent | Chooses the colour that marks what plays. Each theme keeps its own by default. |
| Your own themes | Opens the folder where your theme files go. |
| Reduce effects | Uses solid surfaces instead of artwork colour washes and turns animation off. Smoother on software rendering. |
| Motion artwork | Plays the looping covers some albums and playlists have. Off while Reduce effects is on. Pauses on battery power where Sona can read the power state. |

## Connections

| Setting | What it does | Platform |
| --- | --- | --- |
| Last.fm | Connects your account and adds each song you play to your profile. Builds without a Last.fm key can't connect. | |
| ListenBrainz | Connects with your user token and adds each song to your public history. | |
| Discord status | Shows the song you're playing as your Discord activity. Builds without a Discord application ID don't have it. | |

A song counts as scrobbled once you've played half of it or four minutes. Nothing is scrobbled or shown on Discord while private listening is on. See [Scrobbling and Discord](/guide/scrobbling-and-discord/).

## Shortcuts

Lists every shortcut by group. Select one to record new keys, with a warning when another action already uses them. Use "Filter by action or key" to find one, the reset button beside a shortcut to restore it, and Reset all to restore them all. See [Keyboard shortcuts](/guide/keyboard-shortcuts/).

## Storage

| Setting | What it does | Platform |
| --- | --- | --- |
| On this computer | Shows the size of the artwork cache, the catalogue cache and the library database. | |
| Artwork cache | Limits the covers and backdrops kept on this computer. Above the limit, the artwork shown least recently is removed first. | |
| Catalogue cache | Clear caches removes the Apple Music pages and the artwork cache. Your library and settings stay. | |
| Library database | Compact now gives free space back to the disk. Sona also optimises it when it is idle and nothing is playing. | |
| Settings file | Exports your settings to a file and imports one from another computer. The window size, audio output, rendering fixes and connected accounts stay with this computer. | Needs a file chooser: not on _Windows_ yet. |

## Diagnostics

Runs checks of the playback host, database and keyring when you open it, and reports problems and warnings. Check again repeats them, Copy diagnostics copies a report with tokens and your email address removed, and Open log folder opens the logs. See [Troubleshooting](/guide/troubleshooting/).

To copy one value, such as a version or a path, select it (triple-click takes a line) and press Ctrl+C, or right-click it and choose Copy. On a Mac, press Cmd+C.

| Group | What it shows |
| --- | --- |
| Playback | The playback host, the Widevine module, the sandbox, the audio priority and the stream format. |
| Motion artwork | The video decoders, the power state and the last video and download. Shown where Sona can check them. |
| Credentials | Where Sona stores your tokens: the system keyring or an unencrypted file. |
| Library database | The file, its size, schema, contents, last full sync and the artwork cache. |
| Sona | The version, platform, start-up time and the paths of the log, settings, data and cache. |
| Updates | Previous version, with Restore… to go back to the version from your last update. Shown only when one is kept. |
| Developer tools | Open the inspector. Available only in development builds started with `SONA_INSPECTOR=1`. |
