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
| Greeting on For You | Opens the For You page with "Good morning", "Good afternoon" or "Good evening" in large type, by the time on your computer. On by default. | |
| Use my name | Adds your first name to the greeting ("Good evening, Alex"), taken from your Apple Music profile. It is shown only if you have set up a profile in Apple Music; the name is kept on this computer, refreshed about once a day and removed when you sign out. Where a name is written family name first (Chinese, Japanese, Korean) the whole name is used. On by default; it has no effect while Greeting on For You is off. | |
| Open Apple Music links in Sona | Makes Sona the app that opens `music://` and `itms://` links. Links that start with `https://` stay with your browser. | Needs Sona.app on _macOS_; needs Sona's desktop entry on _Linux_, so not a source build, Flatpak or AppImage that isn't integrated. On _Windows_ an app you chose in Settings → Apps → Default apps, or the Apple Music app, keeps the links until you choose Sona there. |
| First-run setup | Runs "Set up Sona again": the steps of [first-run setup](/guide/first-run/). | |
| When the window is closed | Keeps playing in the background or quits Sona. In the background, Sona stays in the system tray. | Without a tray icon Sona quits. |
| Title bar | Chooses Automatic, Sona's title bar with or without window buttons, or the desktop's. Applies the next time Sona starts. | _Linux_ _Windows_ |
| Window size | Shows the size Sona opens at. Reset size puts it back to the default. | |
| Track notifications | Shows the song and artist, with artwork and a Next button, when a track starts. Not shown while Sona's window is in front. | _macOS_ and _Windows_ show the song and artist only, and a click opens Sona. _macOS_ needs Sona.app. _Linux_ needs a notification service. |
| Stay awake while playing | Stops the computer from suspending on its own while music plays. The screen can still lock and turn off. | On _Linux_ needs logind or the desktop portal. |

### New releases

| Setting | What it does | Platform |
| --- | --- | --- |
| Track new releases from favourite artists | Once a day, looks at the artists you've favourited and shows their new albums, EPs and singles on For You, under [New from Artists You Love](/guide/new-releases/). Off until you turn it on. Releases that are out when you turn it on aren't listed, only the ones that come out afterwards. | |
| Also show releases as desktop notifications | Releases are always in [Notifications](/guide/notifications/): an album you asked to be reminded of when it comes out, and new releases from your artists. This shows them on your desktop too, one for each release or one summary when several arrive together. Click it to open the release. | Needs a notification service on _Linux_ and Sona.app on _macOS_. Not on _Windows_ yet. |

### Updates

| Setting | What it does | Platform |
| --- | --- | --- |
| Check for updates when Sona starts | Asks GitHub for the newest release once the window has opened. Nothing about you or your library is sent. | |
| Show what's new after an update | The first time Sona starts after an update, opens the new version's release notes in a window you can scroll. It asks GitHub for them once the window has opened. It doesn't show on a first install or after going back to an older version, and it shows once for each version. | |
| Release notes | Shows the release notes of the version you're running: press Show. | |
| Test versions | Also offers release candidates and other test versions. | Only where Sona can update itself |

This group also shows whether you're up to date, with What's new, View release, Install and restart and Check now. What's new opens the new version's release notes in a window you can scroll; it is there when the release has notes. It doesn't check development builds. See [Install](/guide/install/) for how updating works.

GitHub lets each network address make only 60 anonymous requests an hour, shared with every other program on that network, and most of Sona's update check doesn't use them. When the limit has been reached anyway, Check now says "GitHub's limit for this network was reached; Sona will check again after" the time it is over, in your clock. Sona asks GitHub nothing until then, for the update check, the release notes and What's new alike, and checks again by itself once the limit is over if "Check for updates when Sona starts" is on.

## Playback

On a platform with no playback engine this section says so, and only Lyrics remains. Browsing Apple Music and your library works as usual.

### Sound

| Setting | What it does | Platform |
| --- | --- | --- |
| Widevine | Shows the state of the Widevine download that plays Apple Music's protected songs, with Try again if it failed. Sona keeps it up to date by itself. | |
| Audio quality | Chooses High (256 kbps AAC) or Data saver (64 kbps). Applies from the next song. | |
| Output | Chooses where Sona plays. Switching moves the music without a pause. | Where the engine can pick a device |
| Sound Check | Plays every song at about the same loudness. | Where the engine supports it |
| Spatial Audio for Headphones | Plays Apple's own headphone mix of songs made in Dolby Atmos. It works for many of them but not all, because Apple keeps some for its own apps; the Spatial tag beside the title shows when it's playing. Off by default, and it applies at once, to the song playing too. | Where the engine supports it |
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
| Mode | Chooses System, Light or Dark. A theme with one variant keeps its look and disables this. On a Mac the window buttons and the see-through sidebar follow it too. |
| Theme | Chooses the colours of the whole window from Sona's own, community palettes and your themes. See [Themes](/guide/themes/). |
| Accent | Chooses the colour that marks what plays. Each theme keeps its own by default. |
| Your own themes | Opens the folder where your theme files go. |
| Reduce effects | Uses solid surfaces instead of artwork colour washes (and, on a Mac or Windows 11, instead of the see-through sidebar) and turns animation off. Smoother on software rendering. |
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
| Settings file | Exports your settings to a file and imports one from another computer. The window size, audio output, rendering fixes and connected accounts stay with this computer. | On _Linux_ needs the desktop portal's file chooser. |

## Diagnostics

Runs checks of the playback host, database and keyring when you open it, and reports problems and warnings. Check again repeats them, Copy diagnostics copies a report for a bug report, and Open log folder opens the logs. The report removes your tokens, email address and user name, and replaces what you played, looked at and searched for with short codes. It keeps your library's counts, your country and the IDs of your add-ons. [Troubleshooting](/guide/troubleshooting/) lists what is removed, replaced and kept.

To copy one value, such as a version or a path, select it (triple-click takes a line) and press Ctrl+C, or right-click it and choose Copy. On a Mac, press Cmd+C.

| Group | What it shows |
| --- | --- |
| Playback | The playback host, the Widevine module, the sandbox, the audio priority and the stream format. |
| Motion artwork | The video decoders, the power state and the last video and download. Shown where Sona can check them. |
| Credentials | Where Sona stores your tokens: the system keyring or an unencrypted file. |
| Library database | The file, its size, schema, contents, last full sync and the artwork cache. |
| Sona | The version, platform, start-up time and the paths of the log, settings, data and cache. |
| Updates | Previous version, with Restore… to go back to the version from your last update. Shown only when one is kept. |
| Developer tools | A switch, off by default: adds **Inspect** to every item's menu, with its IDs, artwork, what Apple sent, and what an add-on receives for it. It is for people who write add-ons; it only reads your own data from Sona's cache and sends nothing anywhere. Below it, Open the inspector (MyGo's element inspector), available only in development builds started with `SONA_INSPECTOR=1`. |
