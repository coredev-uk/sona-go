---
title: Command Line Reference
description: Every sona command, its options, exit status and JSON output.
---

> Control the running Sona from a terminal, script or status bar.

For an introduction, see [Command line](/guide/cli/).

## Options of `sona`

`sona` without a command opens Sona, or hands its Apple Music links to the Sona already running. `sona --hidden` starts without a window, and `sona --version` prints the version.

### `sona.exe --remove-sandbox`

_Windows_

Removes what the playback host's sandbox left on the computer: its AppContainer profile and the access entries granted to it on Sona's, GStreamer's and the Widevine folders. Your settings, library and the downloaded Widevine component stay.

- The installer's uninstaller runs it for you.
- The portable `.zip` has no uninstaller: quit Sona and run it from the folder before you delete the folder.
- It starts nothing and can run again. If a step fails, it exits with status 1 and says why. On other systems it exits with status 1, because there is nothing to remove.

## Commands

Options may come anywhere after the command. `--` ends them, and a number such as `-10` is an argument (`sona volume -10`, `sona seek -15`). Every command takes `-h` and `--help`.

### `sona play`, `pause`, `toggle`, `next`, `prev`

Resume, pause, play or pause, skip to the next song, go to the previous song (or restart the song in its first seconds, like the previous key).

- `sona play <link>` - plays an Apple Music link to a song, album, playlist or station in place of the queue, as its page's Play does. Your library's playlists and albums play from the links `sona library playlists` and `sona library albums` print. Other links (artists, curators, music videos) are a usage error: give them to `sona open`.
- `--shuffle` - with a link, starts the album or playlist shuffled.
- `--force` - `prev` always plays the previous song, however far into this one.
- `--json` - prints the status after the command.

### `sona seek`

Moves to a number of seconds into the playing song, or by `+N` or `-N` seconds from where it is, within the song: `sona seek <seconds>|+N|-N`. Seconds may have decimals.

- `--json` - prints the status.

### `sona shuffle`, `repeat`, `crossfade`

Show or switch them, as the player bar's buttons and Settings → Playback do.

- `sona shuffle [on|off|toggle|status]` and `sona crossfade [on|off|toggle|status]`. The default is `status`. Crossfade keeps the length set in Settings → Playback.
- `sona repeat [off|all|one|cycle|status]`. The default is `status`; `cycle` moves on as the player bar's button does: off, all, one, off.
- `--json` - prints the status.

### `sona status`

Prints the playback state, song, position, volume, shuffle, repeat, crossfade and private listening.

- `--follow` - keeps printing on every change.
- `--format` string (optional) - a template for each line ([Status bars](#status-bars)).
- `--json` - prints the status.

### `sona lyrics`

Prints the playing song's lyrics as plain text.

- `--lrc` - prints an LRC file.
- `--follow` - prints the line being sung, a new one each time it changes ([Lyrics in a bar or shell](#lyrics-in-a-bar-or-shell)).
- `--format` string (optional) - a template for each line.
- `--translation` or `--transliteration` - prints that version of each line.
- `--json` - prints `{"track", "state", "position", "provider", "timing", "synced", "lines"}`. With `--follow`, one `{"running", "state", "track", …}` per line.

### `sona queue`

Prints the queue in play order and marks the playing song.

- `sona queue jump N` - plays the song at place `N`, as the list numbers it (from 1; `index + 1` in the JSON). Prints the status.
- `sona queue clear` - removes every song but the playing one, as the queue's Clear does. Prints the queue.
- `--json` - prints `{"index", "tracks"}`, or the status after `jump`.

### `sona search`

Searches the catalogue in Sona's storefront: `sona search <term>…`.

- `--type` string (optional) - `song`, `album`, `artist`, `playlist`, `station`, `music-video` or `curator`.
- `--limit` number (optional) - results of each kind. Default 5, at most 20.
- `--json` - prints `{"term", "results"}`.

### `sona open`

Opens a `music.apple.com`, `music://` or `itms://` link in Sona's window, as clicking it would: `sona open <link>`.

- `--json` - prints `{"kind", "id", "storefront"}`.

### `sona volume`

Shows the volume, or sets it as the player bar's slider does and keeps it: `sona volume [N|+N|-N]`.

- `--json` - prints `{"volume"}`.

### `sona like`, `dislike`

Rate the playing song. Need an Apple Music account signed in to Sona.

- `sona like [on|off|toggle|status]` - favourites it, which adds it to your library. The default is `on`.
- `sona dislike [on|off|toggle|status]` - rates it Suggest Less, which replaces a favourite. The default is `on`.
- `--json` - prints `{"track", "liked", "disliked", "inLibrary"}`.

### `sona library`

The playing song's place in your library, and your library's lists. Needs an Apple Music account signed in to Sona.

- `sona library [add|remove|toggle|status]` - adds the playing song to your library, removes it, switches it, or shows whether it is there. The default is `status`.
- `sona library playlists`, `sona library albums` - your library's playlists or albums, most recently added first, at most 500, each with the link `sona play` takes.
- `--json` - prints `{"track", "liked", "disliked", "inLibrary"}`, or for the lists `{"kind", "total", "items"}`.

### `sona private`

Shows or sets private listening: `sona private [on|off|toggle|status]`. The default is `status`. `SONA_PRIVATE=1` keeps it on. A development build changes it for the session only.

- `--json` - prints `{"on", "source", "saved"}`.

### `sona addons dev`

_Linux_ for a native add-on; a WebAssembly add-on runs on _Linux_, _macOS_ and _Windows_.

Runs an add-on from a folder without installing it, and prints its log until you press Ctrl+C: `sona addons dev <folder>`. It is for people who write add-ons ([the add-on reference](https://github.com/coredev-uk/sona-go/blob/main/docs/addons.md)). [Sample add-ons](https://github.com/coredev-uk/sona-addons) to run with it are in their own repository.

- Sona asks you to confirm in its window. The question names the add-on, the folder and the permissions its manifest asks for. Without a "Run" there, nothing runs.
- Nothing is saved. The permissions hold for the run. The add-on gets none of your saved values or secrets.
- The folder must be yours and not writable by others. Its ID can't be one of your installed add-ons or one reserved for Sona's own.
- The add-on's log goes to standard output. On a terminal, escape sequences and control characters are stripped from it. Sona's own lines go to standard error.
- It exits with status 0 when you press Ctrl+C or the add-on ends. A refusal is status 1, and no Sona is status 3.
- On _macOS_ and _Windows_ a native add-on fails with "unsupported" before Sona asks anything: they can't yet run the file that was checked. A WebAssembly add-on (`"runtime": "wasm"`) runs there, because Sona reads its `.wasm` once and runs those bytes.

### `sona help`

Prints help for a command: `sona help [command]`.

- `--man` - prints the manual page in roff.

### `sona completion`

Prints the shell completion script: `sona completion bash|zsh|fish`.

## Exit status

| Status | Meaning |
| --- | --- |
| 0 | Success |
| 1 | The command failed in Sona (the message says why: nothing is playing, the song has no lyrics, not signed in, Apple refused, …) |
| 2 | The command line is wrong (unknown command, option or argument, or a link Sona can't open) |
| 3 | Sona isn't running or can't be reached (also while it starts or quits). `status` still prints that it isn't running |

Messages go to standard error, results to standard output.

## JSON

With `--json` a command prints one JSON object on one line. The schemas
(`internal/cli/schema.go`) only ever gain fields; none is renamed, removed
or changes meaning. Every field is always present. Times are seconds, the
volume a percentage.

The status:

```json
{
  "running": true,
  "state": "playing",
  "track": {
    "id": "1441164430",
    "title": "Come Together",
    "artist": "The Beatles",
    "album": "Abbey Road",
    "duration": 259.95,
    "explicit": false,
    "artwork": "https://is1-ssl.mzstatic.com/image/thumb/…/600x600bb.jpg"
  },
  "position": 83.4,
  "volume": 40,
  "shuffle": false,
  "repeat": "off",
  "private": true,
  "preview": false,
  "error": "",
  "liked": true,
  "disliked": false,
  "inLibrary": true,
  "crossfade": false
}
```

- `state`: `idle` (nothing loaded), `loading`, `playing`, `paused`, `ended`
  or `error` (`error` then says what failed).
- `track`: `null` when no song is selected. `id` is the catalogue ID, or the
  library ID (`i.…`) of a song only in your library. `artwork` is a
  600×600 image on Apple's artwork CDN, `""` when there is none.
- `position`: as of the moment the status was taken.
- `repeat`: `off`, `all` or `one`. `preview`: the song plays as its
  30-second preview.
- `liked`, `disliked`, `inLibrary`: the playing song is a favourite, rated
  Suggest Less, in your library, as the player bar shows it. Sona looks it
  up when the song starts, so they are `false` for a moment; they stay
  `false` for live radio and music videos. `crossfade`: Settings →
  Playback's crossfade is on.
- When Sona isn't running, `status --json` prints `"running": false`,
  `"state": "idle"` and every other field empty.

`queue`: `{"index": 1, "tracks": [track…]}`, each track as above plus
`"autoPlay"` (the song came from the station or AutoPlay); `index` is the
playing song's place, `-1` when none. `search`: `{"term": "…", "results":
[{"kind", "id", "name", "subtitle", "explicit", "url"}…]}`, grouped by kind
in Apple's order; `url` is the music.apple.com link, `""` when unknown.
`like`, `dislike` and `library`: `{"track": track, "liked": true,
"disliked": false, "inLibrary": true}`. `library playlists` and `library
albums`: `{"kind": "playlist"|"album", "total": 120, "items": [{"id":
"p.…", "name", "subtitle", "artwork", "url"}…]}`; `total` counts the whole
library, `items` the 500 most recently added, `subtitle` an album's artist,
`url` the link `sona play` takes. `private`: `{"on": true,
"source": "setting"|"development"|"environment", "saved": false}`. `open`:
`{"kind": "album", "id": "1441164426", "storefront": "gb"}`.

`lyrics`: `{"track": track, "state": "playing", "position": 83.4,
"provider": "apple", "timing": "word", "synced": true, "lines": [line…]}`,
and each line `{"start": 12.34, "end": 15.8, "text": "…", "background": "",
"agent": "v1", "part": "Verse", "translation": "", "transliteration": ""}`:

- `provider`: where the lyrics come from: `apple`, `musixmatch`, `lrclib`
  or `netease`, as chosen in Settings → Playback.
- `timing`: `word` (each word timed, which the lines below don't show),
  `line` or `none` (plain text). `synced` is true for `word` and `line`.
  Plain lyrics have `"start": 0, "end": 0` on every line.
- `lines` are in the order sung; timed lines are in order of `start`.
  `start` and `end` are seconds into the song. A line's `end` is at least
  the end of its last word, so it may run past the next line's `start`.
- `background` is the background vocals sung with the line, `agent` the voice
  (`v1`, `v2` for a duet's second voice), `part` the song part where the
  lyrics name them. `translation` and `transliteration` are the line in the
  translation language (Settings → Playback → Lyrics, else the account's)
  and in Latin script; `""` when the line has none.
- `state` and `position` are the playback's when Sona answered.

One line of `lyrics --follow --json`:

```json
{
  "running": true,
  "state": "playing",
  "track": { "id": "1441164430", "title": "Come Together", "…": "…" },
  "position": 12.5,
  "lyrics": "ready",
  "provider": "apple",
  "timing": "word",
  "synced": true,
  "index": 1,
  "line": { "start": 12.34, "end": 15.8, "text": "He come groovin' up slowly", "…": "…" },
  "next": { "start": 16.2, "end": 18, "text": "He got joo joo eyeball", "…": "…" },
  "text": "He come groovin' up slowly"
}
```

- `lyrics`: `ready`, `loading` (the song's lyrics are being fetched) or `none`
  (nothing is playing, the song has no lyrics, or they couldn't be fetched).
- `index`: the place of `line` in the lyrics' `lines`; `-1` and `line: null`
  when no line is sung (before the first, while the voice rests, plain
  lyrics, no lyrics). `next` is the line that begins next, `null` after the
  last.
- `text`: what the text output prints, `""` when no line is sung. With
  `--translation` or `--transliteration` it is that text where the line has
  one; `line` always has all of them.
- `running` is false only in the last line, when Sona has quit; `state`,
  `track` and `position` are as in the status (`position` is when the line
  was printed).

## Status bars

`sona status --follow` prints a line now and another each time the
status changes (the position moving on its own doesn't count), and ends
when Sona quits, after a last line saying it isn't running (exit status 0).
When Sona isn't running at the start it prints that line and exits with 3,
so a bar module should restart it.

`--json` makes each line the status's JSON. `--format TEMPLATE` makes it a
Go [text/template](https://pkg.go.dev/text/template) over the JSON fields
(`{{.state}}`, `{{.track.title}}`), with `{{json …}}` (a JSON value, quoted)
and `{{clock …}}` (seconds as m:ss); use `{{with .track}}…{{end}}`, since
`track` may be `null`.

Waybar (`~/.config/waybar/config`):

```json
"custom/sona": {
  "exec": "sona status --follow --format '{\"text\": {{with .track}}{{json (printf \"%s – %s\" .artist .title)}}{{else}}\"\"{{end}}, \"class\": {{json .state}}, \"alt\": {{json .state}}}'",
  "return-type": "json",
  "restart-interval": 5,
  "on-click": "sona toggle",
  "on-scroll-up": "sona volume +5",
  "on-scroll-down": "sona volume -5"
}
```

Polybar:

```ini
[module/sona]
type = custom/script
exec = sona status --follow --format '{{if eq .state "playing"}}▶{{else}}⏸{{end}} {{with .track}}{{.artist}} – {{.title}}{{end}}'
tail = true
click-left = sona toggle
scroll-up = sona volume +5
scroll-down = sona volume -5
```

## Lyrics in a bar or shell

`sona lyrics` prints the lyrics of the song that is playing, from the
sources chosen in Settings → Playback (the ones the lyrics view uses, with
its cache: asking costs no more requests than the view makes). It needs
Apple Music songs: live radio, music videos and songs only in your library
have no lyrics.

```sh
sona lyrics                      # plain text, one line to a line
sona lyrics --translation        # each line's translation where it has one
sona lyrics --lrc > song.lrc     # an LRC file for a lyrics player
sona lyrics --json | jq '.lines[] | select(.start > 60) | .text'
```

- **Plain text**: one line of text per lyric line, a blank line between
  verses (where the song part changes, or after a pause of 3 seconds or more
  between lines). Background vocals are left out unless they are all a line
  has; a line break inside a line becomes a space.
- **`--lrc`**: `[ti:]`, `[ar:]` and `[al:]` tags, then `[mm:ss.xx]` and the
  line, one per line in time order (hundredths cut off; minutes go past 99
  for a very long song). Word-timed lyrics are written with their line
  times. Lyrics with no timing can't be an LRC file: nothing is printed and
  it exits with status 1 (`these lyrics have no timing…`), so a redirect
  doesn't leave an untimed file looking like LRC; leave `--lrc` out for the
  plain text.
- **`--translation`, `--transliteration`** print the line's translation, or
  its Latin-script sound, _instead of_ the line where it has one, and the
  line itself where it doesn't. They apply to the plain text, `--lrc` and
  `--follow`'s `text`, not to the one-shot `--json` (which carries every
  localization: combining them is an error), and not to each other.
- Nothing playing, a song with no lyrics (`no_lyrics`) and failures end with
  status 1 and a message, as for the other commands; Sona not running is 3.

### Following the line being sung

`sona lyrics --follow` prints a line now and another each time the line
sung changes, until Sona quits (then a last empty line, exit status 0; when
Sona isn't running at the start, an empty line and exit status 3, so a bar
module should restart it, as with `status --follow`). In the text output a
line is the sung line's text, `""` before the first line, while the voice
rests (3 seconds or more between lines: the line goes half a second after it
ends), after the last line, for a song without lyrics (or with plain ones) and
when nothing plays; a line the same as the one before (a pause, say) isn't
printed again. `--json` prints the object above on each change of the line,
the song, the playback state or whether the lyrics are there; `--format
TEMPLATE` is a Go template over those fields, as for `status` (`{{.text}}`,
`{{with .next}}{{.text}}{{end}}`, `{{.index}}`, `{{clock .position}}`).

The line is the one Sona's lyrics view lights: it starts a quarter of a
second before it begins (the eye arrives as the voice does) and stays until
the next one begins. It is worked out in the command from the status's
position, moved on by the clock while the song plays, and it sleeps on one
timer to the next boundary: nothing polls, Sona does no work per line, and
while paused or stopped there is no timer at all. Sona's status stream says
when to look again: a pause, a resume, a song change, and a seek (a jump of
more than half a second in the position, which the stream publishes though
`status --follow` leaves it out as only a position change). The song's
lyrics are fetched once, when the song changes (a fetch that fails other
than for lack of lyrics is told on standard error, and the song shows
nothing until the next one).

Quickshell (a `Process` reading one JSON object per line; it starts again
if Sona wasn't running or quits):

```qml
import QtQuick
import Quickshell
import Quickshell.Io

Scope {
    id: root
    property string line: ""
    property bool playing: false

    Process {
        id: lyrics
        running: true
        command: ["sona", "lyrics", "--follow", "--json"]
        stdout: SplitParser {
            onRead: data => {
                const now = JSON.parse(data)
                root.line = now.text
                root.playing = now.state === "playing"
            }
        }
        onExited: retry.start() // 3 when Sona isn't running, 0 when it quit
    }

    Timer {
        id: retry
        interval: 5000
        onTriggered: lyrics.running = true
    }

    PanelWindow {
        anchors { top: true; left: true; right: true }
        implicitHeight: 32
        exclusionMode: ExclusionMode.Ignore // don't move windows as lines come and go
        visible: root.line !== ""
        Text {
            anchors.centerIn: parent
            text: root.line
            opacity: root.playing ? 1 : 0.5
        }
    }
}
```

Waybar (a custom module; Waybar reads markup, so `escape` is on, and turns
tooltips off in the module's config, not in each line):

```json
"custom/lyrics": {
  "exec": "sona lyrics --follow --format '{\"text\": {{json .text}}, \"class\": {{json .state}}}'",
  "return-type": "json",
  "escape": true,
  "tooltip": false,
  "restart-interval": 5
}
```

Polybar:

```ini
[module/lyrics]
type = custom/script
exec = sona lyrics --follow
tail = true
```

`--translation` and `--transliteration` work with `--follow` too, and
`sona lyrics --lrc > song.lrc` writes the file a lyrics player wants.

## Shell completions and the manual page

The `.deb`, `.rpm` and Arch packages install the bash, zsh and fish completions and the `sona(1)` manual page. Elsewhere, load them by hand:

```sh
source <(sona completion bash)                       # bash, e.g. in ~/.bashrc
sona completion zsh > "${fpath[1]}/_sona"            # zsh
sona completion fish > ~/.config/fish/completions/sona.fish
sona help --man > ~/.local/share/man/man1/sona.1     # then: man sona
```

The AppImage installs neither. Run it with a command: `Sona.AppImage status`.

## How commands reach Sona

- **Linux:** over your D-Bus session bus. The running Sona owns `dev.coredev.Sona`.
- **macOS and Windows:** over a socket, `sona.sock`, in Sona's state folder (`~/Library/Application Support/Sona/State`, `%LOCALAPPDATA%\Sona\State`). Sona answers only processes of your own user.

The window follows every change a command makes. Help, the manual page, completion descriptions, printed lines and errors use Sona's Language setting, else your system's languages. Command and option names, `--json` output and `--format` templates are the same in every language.
