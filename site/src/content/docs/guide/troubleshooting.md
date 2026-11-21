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

1. Choose Copy diagnostics in Settings → Diagnostics. Sona copies a report and removes tokens and your email address. It also leaves out where you are and which hardware you have: the Concerts city, the sidebar's open folders, the audio output device and the text you typed into add-on settings. The report's settings end with a `_withheld` list of what was left out. It also saves a copy in the log folder.
2. Open the [bug report form](https://github.com/coredev-uk/sona-go/issues/new?template=bug_report.yml).
3. Choose your platform and how you installed Sona. Describe what happened and the steps to reproduce it.
4. Paste the diagnostics, and check them for anything personal.

For a question rather than a bug, ask in [Discussions](https://github.com/coredev-uk/sona-go/discussions/categories/q-a).

For a security problem, do not open an issue. See [Privacy and Security](/privacy-and-security/).
