# Command line

`sona <command>` controls the Sona that is running: playback, the volume,
favourites, search, links and private listening, with text for people and
JSON for scripts and status bars. Commands never start Sona.

```sh
sona status                 # what's playing
sona toggle                 # play/pause
sona next; sona prev
sona volume +10             # 0 to 100, or +N/-N
sona like                   # favourite the playing song
sona search abbey road      # results with their links
sona open https://music.apple.com/gb/album/abbey-road/1441164426
sona private on
sona help [command]         # sona(1) has the same
```

Started without a command, `sona` opens Sona, or hands its Apple Music
links to the Sona already running (`sona --hidden` starts without a
window, `sona --version` prints the version).

## Commands

| Command | What it does | `--json` prints |
| --- | --- | --- |
| `play`, `pause`, `toggle`, `next`, `prev` | Resume, pause, play/pause, next song, previous song (or restart the song in its first seconds, like the previous key) | the status after it |
| `status [--follow] [--format T]` | The playback state, song, position, volume, shuffle, repeat and private listening | the status |
| `queue` | The queue in play order, the playing song marked | `{"index", "tracks"}` |
| `search <term>… [--type KIND] [--limit N]` | Catalogue search in Sona's storefront, up to N (default 5, at most 20) results of each kind: `song`, `album`, `artist`, `playlist`, `station`, `music-video`, `curator` | `{"term", "results"}` |
| `open <link>` | Opens a music.apple.com, `music://` or `itms://` link in Sona's window, as clicking it would | `{"kind", "id", "storefront"}` |
| `volume [N\|+N\|-N]` | Shows the volume, or sets it as the player bar's slider does (and keeps it) | `{"volume"}` |
| `like [on\|off\|toggle\|status]` | Favourites the playing song (`on`, the default), un-favourites it, switches it or shows it. Needs an Apple Music account in Sona; favouriting a song adds it to the library | `{"track", "liked"}` |
| `private [on\|off\|toggle\|status]` | Private listening (`status`, the default). `SONA_PRIVATE=1` keeps it on; a development build changes it for the session only | `{"on", "source", "saved"}` |
| `help [command] [--man]` | Help, or the manual page in roff | |
| `completion bash\|zsh\|fish` | The shell completion script | |

Options may come anywhere after the command; `--` ends them, and a number
such as `-10` is an argument (`sona volume -10`). Every command takes
`-h`/`--help`.

### Exit status

| Status | Meaning |
| --- | --- |
| 0 | Success |
| 1 | The command failed in Sona (the message says why: nothing is playing, not signed in, Apple refused, …) |
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
  "error": ""
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
- When Sona isn't running, `status --json` prints `"running": false`,
  `"state": "idle"` and every other field empty.

`queue`: `{"index": 1, "tracks": [track…]}`, each track as above plus
`"autoPlay"` (the song came from the station or AutoPlay); `index` is the
playing song's place, `-1` when none. `search`: `{"term": "…", "results":
[{"kind", "id", "name", "subtitle", "explicit", "url"}…]}`, grouped by kind
in Apple's order; `url` is the music.apple.com link, `""` when unknown.
`like`: `{"track": track, "liked": true}`. `private`: `{"on": true,
"source": "setting"|"development"|"environment", "saved": false}`. `open`:
`{"kind": "album", "id": "1441164426", "storefront": "gb"}`.

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

## Shell completions and the manual page

The deb, rpm and Arch packages install the bash, zsh and fish completions
and `sona(1)`, all generated by the binary from its command table
(`build/linux/Taskfile.yml`, `package:cli`). Elsewhere, load them by hand:

```sh
source <(sona completion bash)                       # bash, e.g. in ~/.bashrc
sona completion zsh > "${fpath[1]}/_sona"            # zsh
sona completion fish > ~/.config/fish/completions/sona.fish
sona help --man > ~/.local/share/man/man1/sona.1     # then: man sona
```

The AppImage and the Flatpak install neither: run the AppImage with a
command (`Sona.AppImage status`), or `flatpak run dev.coredev.Sona status`.

## How it reaches Sona

- Linux: the D-Bus session bus. The running Sona owns `dev.coredev.Sona`
  (its single-instance name) and exports `dev.coredev.Sona.Instance` at
  `/dev/coredev/Sona`: `Run(s request) → (s reply)` answers a command and
  the `StatusChanged(s status)` signal carries each change for `--follow`.
  The session bus is the user's own.
- macOS and Windows: a Unix socket, `sona.sock`, in Sona's state folder
  (`~/Library/Application Support/Sona/State`, `%LOCALAPPDATA%\Sona\State`;
  0700, the socket 0600), held by the running Sona together with an
  exclusive lock on `sona.lock` beside it. Sona answers only processes of
  its own user (the peer's credentials: `LOCAL_PEERCRED` on macOS, the
  token's user SID on Windows) and closes any other connection. The same
  lock and socket give macOS and Windows single instance: a second launch
  hands its links to the running Sona and exits.

Both carry the same protocol (`internal/cli/protocol.go`): a JSON request
`{"command", "args", "kind", "limit"}`, and a reply `{"result"}` or
`{"error": {"code", "message"}}`. The running Sona checks every request
against the command table before acting on it, and acts through the
services the UI uses (`internal/services/commands.go`), so the window
follows every change. All command-line text is in
`internal/cli/messages.go`.
