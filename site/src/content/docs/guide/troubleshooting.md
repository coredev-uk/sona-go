---
title: Troubleshooting
description: Check what Sona needs, find the logs, and report a bug.
---

Start with Diagnostics. It checks what Sona needs and tells you what is wrong.

## Check Sona

1. Open Settings → Diagnostics.
2. Read the verdict. It covers the playback host, the Widevine download, the database and the keyring.
3. Choose Check again after you fix something.

Each problem or warning says what to do. If Sona says your sign-in is stored unencrypted, your system has no working keyring. See [Privacy and Security](/privacy-and-security/).

## Find the logs

Settings → Diagnostics → Open log folder opens the folder. The log is `sona.log`:

| System | Location |
| --- | --- |
| Linux | `~/.local/state/sona/` (or `$XDG_STATE_HOME/sona/`) |
| macOS | `~/Library/Application Support/Sona/State/` |
| Windows | `%LOCALAPPDATA%\Sona\State\` |

## Report a bug

1. Choose Copy diagnostics in Settings → Diagnostics. Sona copies a report that has been made anonymous (see below) and saves a copy in the log folder.
2. Open the [bug report form](https://github.com/coredev-uk/sona-go/issues/new?template=bug_report.yml).
3. Choose your platform and how you installed Sona. Describe what happened and the steps to reproduce it.
4. Paste the diagnostics, and check them for anything personal.

### What the report removes, replaces and keeps

The report is made anonymous before it reaches the clipboard, whichever line or section holds the data.

| | What |
| --- | --- |
| Removed | Tokens and sign-in credentials. Your email address. Your user name and home folder (shown as `~`). Where you are and which hardware you have: the Concerts city, the sidebar's open folders, the audio output device and the text you typed into add-on settings (the report's settings end with a `_withheld` list of what was left out). The query of every web address (search terms, IDs and credentials). Names of files under your Music, Documents, Downloads and other personal folders, and of any song file (shown as `~/Music/{path}`). |
| Replaced | What you played, looked at and searched for in the log: the IDs, titles, names and search terms of songs, albums, artists, playlists, stations and shelves, Apple's IDs (such as `i.…`, `pl.…`, `ra.…`) and cache keys. Each becomes a code like `id~3fa2ab`. The same song has the same code everywhere in one report, so a report can show that one song failed twice, but not which song. The next report uses other codes, so two reports cannot be matched. Web addresses on Apple's servers keep their host and the shape of the request (`/v1/me/recommendations/{id}`) with every ID shown as `{id}`; other addresses keep their host and first path segment. A start page or other route in the settings keeps its page type (`/playlist/{id}`). |
| Kept | The counts of your library (songs, albums, artists and playlists), your country (the storefront), and the IDs of your add-ons, because catalogue add-ons are public. Versions, sizes, durations, counts, true or false flags and error codes. The operating system, the desktop, the settings that are not listed above, and Sona's own folders. |

For a question rather than a bug, ask in [Discussions](https://github.com/coredev-uk/sona-go/discussions/categories/q-a).

For a security problem, do not open an issue. See [Privacy and Security](/privacy-and-security/).
