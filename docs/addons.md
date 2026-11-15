# Add-ons: protocol v2

An add-on is a program that runs beside Sona, hears what plays and may ask Sona to do small things. It talks to Sona over **protocol v2**: one JSON object per line, in both directions, over the add-on's standard input and output. This page documents every message. The Go side is `internal/cli` (`addon.go`, `addon_session.go`); the plan is `docs/maintainers/proposals/addons-and-remaining-work.md`.

An add-on can be written in any language. `internal/cli/testdata/addon-client.sh` (shell) and `addon-client.py` (Python) are two complete clients; Sona's tests run both.

> **Permissions are advisory until the OS sandbox (#304) lands.** An add-on is a program that runs with your rights. It can read what you can read, including files and the keyring, and it can reach the network. Permissions limit what Sona's protocol hands it; they do not limit the process. Run only add-ons you trust, and say so wherever one is offered for approval.

Status: the protocol, manifests and the supervisor that starts add-ons are implemented. UI contributions, Settings → Add-ons and `sona addons dev` follow in their own change (#300).

## Installing an add-on

An add-on is a folder in the `addons` directory of Sona's config directory (`~/.config/sona/addons` on Linux). Sona creates the directory with mode 0700: an add-on is code Sona runs, so only you add one. The folder's name is the add-on's ID, and it holds the manifest, `addon.json`, and the program the manifest names.

```json example Manifest
{"id":"now-playing-webhook","name":"Now playing webhook","version":"1.2.0","description":"Posts the song that plays to a web address.","command":"bin/webhook","permissions":["playback.read"]}
```

| Field | Rules |
| --- | --- |
| `id` | Required. Lowercase letters, digits and hyphens, starting with a letter, 2 to 40 characters, no dots. It equals the folder's name (compared without regard to case). `lastfm`, `listenbrainz`, `discord` and anything starting with `sona` are reserved for Sona's own bundled add-ons and refused in your folder. |
| `name` | Required. 1 to 60 characters of plain text: no control, bidirectional or other format characters, no space at either end. Shown wherever the add-on contributes something. |
| `version` | Required. Semver (`1.2.0`). |
| `description` | Optional. At most 300 characters, plain text like `name`. |
| `command` | Required. The program to run, a path inside the folder: relative, `/` separators, no `..`, no `.` or empty element, no drive or backslash, at most 200 bytes. |
| `permissions` | Required, may be empty. The [permissions](#permissions) the add-on asks for, each at most once. |

The manifest is at most 64 KiB, and **a field this version doesn't know refuses it**. Why an add-on is refused is one of these codes, shown in Settings: `unreadable`, `manifest_too_large`, `manifest_invalid`, `bad_id`, `reserved_id`, `id_mismatch`, `bad_name`, `bad_version`, `bad_command`, `bad_permission`, `no_executable`, `executable_escapes`, `unsafe_owner`, `too_many_addons`. At most 100 folders are read.

**Safety rules for the files.** Sona refuses an add-on when:

- the folder is a link, or the program, once every link in its path is followed, is not inside the folder (`executable_escapes`);
- the program is not a regular file, or on Linux and macOS not executable (`no_executable`);
- on Linux and macOS, the folder, the manifest, the program or any folder on the way to it belongs to another user, or is writable by its group or by everyone (`unsafe_owner`). Windows keeps a profile's folders to their user by ACLs, which Sona doesn't read.

Sona runs the program by the absolute path it checked: never through `PATH` or the working directory.

### Permissions are granted when you enable an add-on

Enabling an add-on shows the permissions its manifest asks for and, once you approve them, saves them as granted. The grant is yours and stays on this computer: a settings export leaves it out and a settings import never sets it. An update to the add-on that **asks for another permission** does not run until you approve again. Revoking a permission refuses the add-on's very next call, and the add-on needs approval before it runs again. And because permissions are advisory until the OS sandbox (#304), enabling an add-on is trusting it as you trust any program you run.

### How Sona runs an add-on

- **After the first paint**, never during start-up. A hello must arrive within 10 seconds, or the add-on is killed.
- **Its environment** is an allowlist: `HOME`, `PATH`, `LANG`, `LANGUAGE`, `LC_*`, `TZ`, `TMPDIR` (and on Windows what a process needs to start: `SystemRoot`, `SystemDrive`, `TEMP`, `TMP`, `USERPROFILE`, `PATHEXT`, `ComSpec`), plus `SONA_ADDON_ID` and `SONA_ADDON_PROTOCOL=2`. A token, a key, a bus address or anything else in Sona's environment never reaches it. Its working directory is its folder.
- **Its processes end with Sona's.** An add-on leads a process group of its own (a job object on Windows). When Sona quits, it sends the `shutdown` event, closes the add-on's input, gives it a second, asks it to terminate, gives it a second more, then kills the whole group and reaps it. On Linux the kernel also kills the add-on if Sona dies without quitting. macOS has no such signal: when Sona is killed the add-on's input closes, and **an add-on must exit when its input ends**. Do not start children that leave the group.
- **It is restarted when it stops on its own**, after 1, 2, 5, 15 and then 30 seconds. After 5 stops in a row, with no run of a minute between, it is switched off and Sona tells you.
- **Its log** is `addons/<id>.log` in Sona's state directory, beside `sona.log`: what it writes to standard error, with the time, control characters replaced by spaces and lines cut at 4 KiB, and Sona's own notes marked `sona:`. The file is mode 0600, rotated at 1 MiB with one older copy kept. The bug-report bundle lists add-ons (ID, version, on or off) and never their logs.

## The channel

- **It is inherited, not found.** The supervisor starts the add-on with the channel as its standard input and output (the add-on's log goes to its standard error). There is no socket to connect to and no name to guess. The D-Bus bus and the command socket that carry the `sona` CLI's v1 commands refuse every v2 method (`hello` included), and the add-on channel refuses v1.
- **One frame per line**, UTF-8 JSON, ended by `\n`. A frame is at most **1 MiB** (the limit is in the `hello` answer). A longer line ends the session.
- **Requests have an `id`** (an integer from 1) and a `method`. Their answer has the same `id` and a `result` or an `error`. IDs of the two directions are separate: Sona's `invoke` requests count on their own.
- **Events** have an `event` and `data` and no `id`; nobody answers them.
- **Unknown fields are ignored** in what Sona sends: a minor version may add fields, never rename, remove or change one. An add-on must do the same.

## The session

1. The add-on sends [`hello`](#hello) as its first frame. Anything else first (a v1 command, a `call`, text that isn't JSON) is answered once with `hello_required` and the channel is closed.
2. It [subscribes](#subscribe) to topics and receives [events](#events); it sends [`call`](#call)s; Sona [`invoke`](#invoke)s it.
3. When Sona stops the add-on it sends the [`shutdown`](#shutdown) event, then closes the channel after a short grace.

## Permissions

An add-on names the permissions it wants in `hello`; the user grants some of them. **Every call and every event checks the grant again**: revoking a permission refuses the very next call and stops the events at the next change. The `hello` answer says only what held at that moment.

| Permission | What it allows |
| --- | --- |
| `playback.read` | Topics `status` and `now-playing`; the `status` call. |
| `playback.control` | `play`, `pause`, `toggle`, `next`, `prev`. |
| `queue.read` | Topic `queue`; the `queue` call. |
| `lyrics.read` | The `lyrics` call. |
| `search` | The `search` call (Apple's catalogue; rate limited). |
| `open` | The `open` call: Apple Music links only, as the CLI's `open`. |
| `volume` | The `volume` call. |
| `like` | The `like` call (rate limited). |
| `private.read` | The `private` call, to read whether private listening is on. |

Every v1 command maps to exactly one permission, and an add-on may send only the commands in this table; any other (`help`, `completion`, a typo) is `unknown_command`. **An add-on can never switch private listening off or on**: `private` takes no argument but `status`, whatever is granted.

## Private listens and previews

The user's private listening is the user's alone. For a **private** listen or a **30-second preview**, an add-on gets:

- no event: the `now-playing` topic says nothing is playing, the `status` topic says `idle`, the `queue` topic is empty;
- reads that answer as if nothing played: `status` is idle, `queue` is empty, `lyrics` and `like` fail with `nothing_playing`;
- the same after private listening is switched **off** again: a listen that was private at any point since it started stays hidden until that listen ends. Turning private mode on in the middle of a listen is not recalled: what was already sent (its start) was already sent.

Sona judges this once, in one place, with the time the listen started. An add-on is also never told `private: true` or `preview: true` in a status: those are always `false`, because a listen it may see is public.

## Messages

All schemas below are one [JSON Schema](https://json-schema.org/) document (draft 2020-12; the `$defs` are what the examples cite). Sona's tests validate every example in this page, and every frame the session sends, against it.

### `hello`

The first frame of every session. `protocol` is the range of protocol versions the add-on speaks (`major.minor` or `major.minor.patch`; the patch is ignored). Sona answers with the **highest version both speak** and refuses with `version_unsupported` when there is none (an add-on whose minimum is above Sona's highest, or whose maximum is below Sona's lowest). `id` is the add-on's ID, which must be the one the supervisor started it as; a `hello` naming another add-on is refused with `bad_hello`, and so is one asking for a permission this protocol doesn't have. Sona currently speaks `2.0.0` only.

```json example HelloRequest
{"id":1,"method":"hello","params":{"id":"now-playing-webhook","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":["playback.read","playback.control"]}}
```

The answer lists the asked-for permissions that are granted now, and the session's limits.

```json example HelloResponse
{"id":1,"result":{"id":"now-playing-webhook","protocol":"2.0.0","permissions":["playback.read"],"limits":{"maxFrame":1048576,"calls":8,"invokes":4,"subscriptions":8,"args":32,"argBytes":4096,"queue":256}}}
```

A second `hello` is answered `already_hello`.

### `subscribe` and `unsubscribe`

Both take `topics` and answer with the topics subscribed to afterwards. A subscription needs the topic's permission (`status` and `now-playing`: `playback.read`; `queue`: `queue.read`). At most 8 topics may be subscribed to at once. Subscribing delivers the topic's latest value at once, then every change. Subscribing again to a topic is harmless.

```json example SubscribeRequest
{"id":2,"method":"subscribe","params":{"topics":["status","now-playing"]}}
```

```json example TopicsResponse
{"id":2,"result":{"topics":["status","now-playing"]}}
```

```json example UnsubscribeRequest
{"id":3,"method":"unsubscribe","params":{"topics":["status"]}}
```

### Events

| Event | Data | When |
| --- | --- | --- |
| `status` | [`Status`](#status) | Any change of playback, volume, shuffle or repeat, and a position that jumps (a seek). A position that only moves on is not an event; compute it from `position` and the state. |
| `now-playing` | [`NowPlaying`](#nowplaying) | The song that plays changes, and when nothing plays. |
| `queue` | [`Queue`](#queue) | The queue changes. At most 1000 songs, a window around the playing one. |
| `shutdown` | none | Sent to every session, subscribed or not: Sona is stopping the add-on. |

**A slow reader never slows Sona.** Each session keeps only the newest event of each topic: an add-on that reads slowly sees the latest state, not every step. An add-on that lets more than 256 frames wait unread is ended.

```json example StatusEvent
{"event":"status","data":{"running":true,"state":"playing","track":{"id":"1441164430","title":"Come Together","artist":"The Beatles","album":"Abbey Road","duration":259.0,"explicit":false,"artwork":"https://is1-ssl.mzstatic.com/image/thumb/Music/600x600bb.jpg"},"position":12.5,"volume":80,"shuffle":false,"repeat":"off","private":false,"preview":false,"error":""}}
```

```json example NowPlayingEvent
{"event":"now-playing","data":{"track":{"id":"1441164430","title":"Come Together","artist":"The Beatles","album":"Abbey Road","duration":259.0,"explicit":false,"artwork":"https://is1-ssl.mzstatic.com/image/thumb/Music/600x600bb.jpg"},"startedAt":1767225600000}}
```

```json example NowPlayingNothingEvent
{"event":"now-playing","data":{"track":null,"startedAt":0}}
```

```json example QueueEvent
{"event":"queue","data":{"index":0,"tracks":[{"id":"1441164430","title":"Come Together","artist":"The Beatles","album":"Abbey Road","duration":259.0,"explicit":false,"artwork":"","autoPlay":false}]}}
```

```json example ShutdownEvent
{"event":"shutdown"}
```

**Only public links.** In events and answers, `artwork` is an https image on Apple's artwork CDN (`*.mzstatic.com`) or empty, and a search result's `url` is a `music.apple.com` link or empty. An add-on never sees an Apple API host, a token, audio or video.

### `call`

Runs one of the `sona` CLI's v1 commands (`site/src/content/docs/reference/cli.md`) for the add-on, as the running Sona answers it. `params` is the v1 request: `command`, `args`, and for `search` `kind` and `limit`. Only the commands with a permission above are accepted, and not the streams (`--follow`). The `result` is that command's JSON: [`Status`](#status) for `status`, `play`, `pause`, `toggle`, `next` and `prev`; [`Queue`](#queue); [`SearchResults`](#searchresults); [`Lyrics`](#lyrics); [`Like`](#like); [`Volume`](#volume); [`Private`](#private); [`Opened`](#opened). The commands that act on the playing song (`lyrics`, `like`) fail with `nothing_playing` for a hidden listen.

```json example CallRequest
{"id":4,"method":"call","params":{"command":"search","args":["abbey road"],"kind":"album","limit":5}}
```

```json example SearchResponse
{"id":4,"result":{"term":"abbey road","results":[{"kind":"album","id":"1441164426","name":"Abbey Road","subtitle":"The Beatles","explicit":false,"url":"https://music.apple.com/gb/album/abbey-road/1441164426"}]}}
```

```json example StatusResponse
{"id":5,"result":{"running":true,"state":"paused","track":null,"position":0,"volume":80,"shuffle":false,"repeat":"off","private":false,"preview":false,"error":""}}
```

`open` keeps the CLI's rules: only Apple Music links Sona can open (`bad_link` otherwise).

### `invoke`

The one request Sona sends. When the user chooses one of the add-on's contributions (a palette command, an item action), Sona sends `invoke`. `action` is the contribution's ID (`addon.<id>.<name>`), `target` what it was chosen on (or `null`), and `deadlineMs` the time the add-on has to answer: **Sona sets the deadline** (10 seconds by default, never more than 30), and a late answer is dropped. At most 4 invokes may wait for an answer at once; more fail with `busy`.

```json example InvokeRequest
{"id":1,"method":"invoke","params":{"action":"addon.now-playing-webhook.send","target":{"kind":"album","id":"1441164426","name":"Abbey Road"},"deadlineMs":10000}}
```

The add-on answers with the same `id`. Both fields of the result are plain text and optional: `toast` (at most 500 bytes) is shown by Sona, `url` (http or https, no credentials, at most 2048 bytes) is offered to the user to open and opens only after a click. Anything else makes the invoke fail.

```json example InvokeResponse
{"id":1,"result":{"toast":"Sent","url":"https://example.com/played"}}
```

An add-on that cannot do it answers with an `error`; Sona passes only its `code` on.

### Errors

An error frame carries a code and **a fixed message** for that code. It never carries the text of the error that caused it: that text could hold a URL, a path or a token. Switch on `code`; the message is for people.

```json example ErrorResponse
{"id":4,"error":{"code":"permission_denied","message":"the add-on doesn't have the permission for this"}}
```

| Code | Meaning |
| --- | --- |
| `bad_frame` | The frame is not a JSON object of the protocol. |
| `frame_too_large` | A frame is longer than the limit; the session ends. |
| `hello_required` | The first frame must be `hello`; the session ends. |
| `already_hello` | `hello` was already sent. |
| `bad_hello` | `hello` is not valid, or names another add-on; the session ends. |
| `version_unsupported` | No protocol version both sides speak; the session ends. |
| `unknown_method` | No such method. |
| `unknown_topic` | No such topic. |
| `permission_denied` | The add-on doesn't hold the permission. |
| `rate_limited` | Too many calls; slow down. |
| `busy` | Too many calls (or invokes) at once. |
| `too_many_subscriptions` | Over the subscription limit. |
| `bad_arguments` | The arguments are not valid for the command. |
| `result_too_large` | The answer is longer than the frame limit. |
| `timeout` | The call (or invoke) took too long. |
| `not_running` | The add-on is not running. |
| `failed` | Sona couldn't do that. Everything with no code of its own is this. |
| `unknown_command` | No such command (or one an add-on may not send). |
| `nothing_playing` | Nothing is playing (or nothing an add-on may see). |
| `no_lyrics` | The song has no lyrics. |
| `bad_link` | Not an Apple Music link Sona can open. |
| `account_loading` | Sona is still reading the account. |
| `starting` | Sona is starting. |
| `shutting_down` | Sona is shutting down. |
| `not_signed_in` | Needs an Apple Music account signed in to Sona. |
| `network` | Sona couldn't reach the network. |
| `unsupported` | Not available on this system. |
| `private_forced` | Private listening is forced on. |
| `not_found` | It couldn't be found. |
| `not_playable` | The song isn't available to stream. |
| `no_subscription` | The account has no Apple Music subscription. |
| `live_station` | Not available while a live station plays. |
| `station` | Not available while a station plays. |
| `not_released` | The song isn't out yet. |
| `unauthorized` | Apple Music refused the request. |
| `cancelled` | The call was cancelled. |

## Limits

Sona reports its limits in the `hello` answer and enforces each. A refusal is an error frame, or ends the session where the table says so.

| Limit | Value | Over it |
| --- | --- | --- |
| Frame size | 1 MiB | `frame_too_large`, session ends |
| Calls in flight | 8 | `busy` |
| Invokes awaiting an answer | 4 | `busy` |
| Subscriptions | 8 topics | `too_many_subscriptions` |
| Arguments of a call | 32, each at most 4096 bytes | `bad_arguments` |
| Unread frames queued for the add-on | 256 | session ends |
| Call rate | 20 per second, burst 40 | `rate_limited` |
| `search` | 2 per second, burst 5 | `rate_limited` |
| `like`, `open` | 1 per second, burst 3 | `rate_limited` |

## Schema

```json schema
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/coredev-uk/sona-go/docs/addons.md",
  "$defs": {
    "Permission": {"enum": ["playback.read", "playback.control", "queue.read", "lyrics.read", "search", "open", "volume", "like", "private.read"]},
    "Topic": {"enum": ["status", "now-playing", "queue"]},
    "Version": {"type": "string", "pattern": "^(0|[1-9][0-9]{0,5})\\.(0|[1-9][0-9]{0,5})(\\.(0|[1-9][0-9]{0,5}))?$"},
    "Error": {
      "type": "object", "additionalProperties": false, "required": ["code", "message"],
      "properties": {"code": {"type": "string", "pattern": "^[a-z_]{1,40}$"}, "message": {"type": "string", "maxLength": 200}}
    },
    "ErrorResponse": {
      "type": "object", "additionalProperties": false, "required": ["id", "error"],
      "properties": {"id": {"type": "integer", "minimum": 0}, "error": {"$ref": "#/$defs/Error"}}
    },
    "Artwork": {"type": "string", "pattern": "^(https://[a-z0-9.-]+\\.mzstatic\\.com(:443)?/[^?#@ ]*)?$", "maxLength": 2048},
    "ShareURL": {"type": "string", "pattern": "^(https://music\\.apple\\.com(:443)?/[^#@ ]*)?$", "maxLength": 2048},
    "Track": {
      "type": "object", "additionalProperties": false,
      "required": ["id", "title", "artist", "album", "duration", "explicit", "artwork"],
      "properties": {
        "id": {"type": "string"}, "title": {"type": "string"}, "artist": {"type": "string"}, "album": {"type": "string"},
        "duration": {"type": "number", "minimum": 0}, "explicit": {"type": "boolean"}, "artwork": {"$ref": "#/$defs/Artwork"}
      }
    },
    "QueueTrack": {
      "type": "object", "additionalProperties": false,
      "required": ["id", "title", "artist", "album", "duration", "explicit", "artwork", "autoPlay"],
      "properties": {
        "id": {"type": "string"}, "title": {"type": "string"}, "artist": {"type": "string"}, "album": {"type": "string"},
        "duration": {"type": "number", "minimum": 0}, "explicit": {"type": "boolean"}, "artwork": {"$ref": "#/$defs/Artwork"},
        "autoPlay": {"type": "boolean"}
      }
    },
    "Status": {
      "type": "object", "additionalProperties": false,
      "required": ["running", "state", "track", "position", "volume", "shuffle", "repeat", "private", "preview", "error"],
      "properties": {
        "running": {"type": "boolean"},
        "state": {"enum": ["idle", "loading", "playing", "paused", "ended", "error"]},
        "track": {"oneOf": [{"type": "null"}, {"$ref": "#/$defs/Track"}]},
        "position": {"type": "number", "minimum": 0},
        "volume": {"type": "integer", "minimum": 0, "maximum": 100},
        "shuffle": {"type": "boolean"},
        "repeat": {"enum": ["off", "all", "one"]},
        "private": {"const": false},
        "preview": {"const": false},
        "error": {"type": "string"}
      }
    },
    "NowPlaying": {
      "type": "object", "additionalProperties": false, "required": ["track", "startedAt"],
      "properties": {
        "track": {"oneOf": [{"type": "null"}, {"$ref": "#/$defs/Track"}]},
        "startedAt": {"type": "integer", "minimum": 0}
      }
    },
    "Queue": {
      "type": "object", "additionalProperties": false, "required": ["index", "tracks"],
      "properties": {
        "index": {"type": "integer", "minimum": -1},
        "tracks": {"type": "array", "maxItems": 1000, "items": {"$ref": "#/$defs/QueueTrack"}}
      }
    },
    "SearchResults": {
      "type": "object", "additionalProperties": false, "required": ["term", "results"],
      "properties": {
        "term": {"type": "string"},
        "results": {"type": "array", "items": {
          "type": "object", "additionalProperties": false,
          "required": ["kind", "id", "name", "subtitle", "explicit", "url"],
          "properties": {
            "kind": {"enum": ["song", "album", "artist", "playlist", "station", "music-video", "curator"]},
            "id": {"type": "string"}, "name": {"type": "string"}, "subtitle": {"type": "string"},
            "explicit": {"type": "boolean"}, "url": {"$ref": "#/$defs/ShareURL"}
          }
        }}
      }
    },
    "LyricLine": {
      "type": "object", "additionalProperties": false,
      "required": ["start", "end", "text", "background", "agent", "part", "translation", "transliteration"],
      "properties": {
        "start": {"type": "number"}, "end": {"type": "number"}, "text": {"type": "string"},
        "background": {"type": "string"}, "agent": {"type": "string"}, "part": {"type": "string"},
        "translation": {"type": "string"}, "transliteration": {"type": "string"}
      }
    },
    "Lyrics": {
      "type": "object", "additionalProperties": false,
      "required": ["track", "state", "position", "provider", "timing", "synced", "lines"],
      "properties": {
        "track": {"$ref": "#/$defs/Track"}, "state": {"type": "string"}, "position": {"type": "number"},
        "provider": {"type": "string"}, "timing": {"enum": ["word", "line", "none"]}, "synced": {"type": "boolean"},
        "lines": {"type": "array", "items": {"$ref": "#/$defs/LyricLine"}}
      }
    },
    "Like": {
      "type": "object", "additionalProperties": false, "required": ["track", "liked"],
      "properties": {"track": {"$ref": "#/$defs/Track"}, "liked": {"type": "boolean"}}
    },
    "Volume": {
      "type": "object", "additionalProperties": false, "required": ["volume"],
      "properties": {"volume": {"type": "integer", "minimum": 0, "maximum": 100}}
    },
    "Private": {
      "type": "object", "additionalProperties": false, "required": ["on", "source", "saved"],
      "properties": {"on": {"type": "boolean"}, "source": {"enum": ["setting", "development", "environment"]}, "saved": {"type": "boolean"}}
    },
    "Opened": {
      "type": "object", "additionalProperties": false, "required": ["kind", "id", "storefront"],
      "properties": {"kind": {"type": "string"}, "id": {"type": "string"}, "storefront": {"type": "string"}}
    },
    "Limits": {
      "type": "object", "additionalProperties": false,
      "required": ["maxFrame", "calls", "invokes", "subscriptions", "args", "argBytes", "queue"],
      "properties": {
        "maxFrame": {"type": "integer", "minimum": 1}, "calls": {"type": "integer", "minimum": 1},
        "invokes": {"type": "integer", "minimum": 1}, "subscriptions": {"type": "integer", "minimum": 1},
        "args": {"type": "integer", "minimum": 1}, "argBytes": {"type": "integer", "minimum": 1},
        "queue": {"type": "integer", "minimum": 1}
      }
    },
    "HelloRequest": {
      "type": "object", "additionalProperties": false, "required": ["id", "method", "params"],
      "properties": {
        "id": {"type": "integer", "minimum": 1},
        "method": {"const": "hello"},
        "params": {
          "type": "object", "additionalProperties": false, "required": ["id", "protocol", "permissions"],
          "properties": {
            "id": {"type": "string", "pattern": "^[a-z][a-z0-9-]{1,39}$"},
            "protocol": {
              "type": "object", "additionalProperties": false, "required": ["min", "max"],
              "properties": {"min": {"$ref": "#/$defs/Version"}, "max": {"$ref": "#/$defs/Version"}}
            },
            "permissions": {"type": "array", "maxItems": 9, "uniqueItems": true, "items": {"$ref": "#/$defs/Permission"}}
          }
        }
      }
    },
    "HelloResponse": {
      "type": "object", "additionalProperties": false, "required": ["id", "result"],
      "properties": {
        "id": {"type": "integer", "minimum": 1},
        "result": {
          "type": "object", "additionalProperties": false, "required": ["id", "protocol", "permissions", "limits"],
          "properties": {
            "id": {"type": "string"}, "protocol": {"$ref": "#/$defs/Version"},
            "permissions": {"type": "array", "uniqueItems": true, "items": {"$ref": "#/$defs/Permission"}},
            "limits": {"$ref": "#/$defs/Limits"}
          }
        }
      }
    },
    "SubscribeRequest": {
      "type": "object", "additionalProperties": false, "required": ["id", "method", "params"],
      "properties": {
        "id": {"type": "integer", "minimum": 1}, "method": {"enum": ["subscribe", "unsubscribe"]},
        "params": {
          "type": "object", "additionalProperties": false, "required": ["topics"],
          "properties": {"topics": {"type": "array", "maxItems": 8, "items": {"$ref": "#/$defs/Topic"}}}
        }
      }
    },
    "UnsubscribeRequest": {"$ref": "#/$defs/SubscribeRequest"},
    "TopicsResponse": {
      "type": "object", "additionalProperties": false, "required": ["id", "result"],
      "properties": {
        "id": {"type": "integer", "minimum": 1},
        "result": {
          "type": "object", "additionalProperties": false, "required": ["topics"],
          "properties": {"topics": {"type": "array", "maxItems": 8, "uniqueItems": true, "items": {"$ref": "#/$defs/Topic"}}}
        }
      }
    },
    "CallRequest": {
      "type": "object", "additionalProperties": false, "required": ["id", "method", "params"],
      "properties": {
        "id": {"type": "integer", "minimum": 1}, "method": {"const": "call"},
        "params": {
          "type": "object", "additionalProperties": false, "required": ["command"],
          "properties": {
            "command": {"enum": ["play", "pause", "toggle", "next", "prev", "status", "queue", "lyrics", "search", "open", "volume", "like", "private"]},
            "args": {"type": "array", "maxItems": 32, "items": {"type": "string", "maxLength": 4096}},
            "kind": {"enum": ["song", "album", "artist", "playlist", "station", "music-video", "curator"]},
            "limit": {"type": "integer", "minimum": 1, "maximum": 20}
          }
        }
      }
    },
    "StatusResponse": {
      "type": "object", "additionalProperties": false, "required": ["id", "result"],
      "properties": {"id": {"type": "integer", "minimum": 1}, "result": {"$ref": "#/$defs/Status"}}
    },
    "SearchResponse": {
      "type": "object", "additionalProperties": false, "required": ["id", "result"],
      "properties": {"id": {"type": "integer", "minimum": 1}, "result": {"$ref": "#/$defs/SearchResults"}}
    },
    "CallResponse": {
      "type": "object", "additionalProperties": false, "required": ["id", "result"],
      "properties": {
        "id": {"type": "integer", "minimum": 1},
        "result": {"oneOf": [
          {"$ref": "#/$defs/Status"}, {"$ref": "#/$defs/Queue"}, {"$ref": "#/$defs/SearchResults"}, {"$ref": "#/$defs/Lyrics"},
          {"$ref": "#/$defs/Like"}, {"$ref": "#/$defs/Volume"}, {"$ref": "#/$defs/Private"}, {"$ref": "#/$defs/Opened"}
        ]}
      }
    },
    "StatusEvent": {
      "type": "object", "additionalProperties": false, "required": ["event", "data"],
      "properties": {"event": {"const": "status"}, "data": {"$ref": "#/$defs/Status"}}
    },
    "NowPlayingEvent": {
      "type": "object", "additionalProperties": false, "required": ["event", "data"],
      "properties": {"event": {"const": "now-playing"}, "data": {"$ref": "#/$defs/NowPlaying"}}
    },
    "NowPlayingNothingEvent": {"$ref": "#/$defs/NowPlayingEvent"},
    "QueueEvent": {
      "type": "object", "additionalProperties": false, "required": ["event", "data"],
      "properties": {"event": {"const": "queue"}, "data": {"$ref": "#/$defs/Queue"}}
    },
    "ShutdownEvent": {
      "type": "object", "additionalProperties": false, "required": ["event"],
      "properties": {"event": {"const": "shutdown"}}
    },
    "InvokeRequest": {
      "type": "object", "additionalProperties": false, "required": ["id", "method", "params"],
      "properties": {
        "id": {"type": "integer", "minimum": 1}, "method": {"const": "invoke"},
        "params": {
          "type": "object", "additionalProperties": false, "required": ["action", "target", "deadlineMs"],
          "properties": {
            "action": {"type": "string", "pattern": "^addon\\.[a-z][a-z0-9-]{1,39}\\.[a-z0-9._-]{1,64}$"},
            "target": {"oneOf": [{"type": "null"}, {"type": "object"}]},
            "deadlineMs": {"type": "integer", "minimum": 1, "maximum": 30000}
          }
        }
      }
    },
    "InvokeResponse": {
      "type": "object", "additionalProperties": false, "required": ["id", "result"],
      "properties": {
        "id": {"type": "integer", "minimum": 1},
        "result": {
          "type": "object", "additionalProperties": false,
          "properties": {
            "toast": {"type": "string", "maxLength": 500},
            "url": {"type": "string", "pattern": "^https?://[^@ ]+$", "maxLength": 2048}
          }
        }
      }
    },
    "Manifest": {
      "type": "object", "additionalProperties": false, "required": ["id", "name", "version", "command", "permissions"],
      "properties": {
        "id": {"type": "string", "pattern": "^[a-z][a-z0-9-]{1,39}$"},
        "name": {"type": "string", "minLength": 1, "maxLength": 240},
        "version": {"type": "string", "pattern": "^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\\+[0-9A-Za-z.-]+)?$", "maxLength": 40},
        "description": {"type": "string", "maxLength": 1200},
        "command": {"type": "string", "pattern": "^[^/\\\\:][^\\\\:]*$", "maxLength": 200},
        "permissions": {"type": "array", "maxItems": 9, "uniqueItems": true, "items": {"$ref": "#/$defs/Permission"}}
      }
    },
    "Frame": {
      "oneOf": [
        {"$ref": "#/$defs/HelloRequest"}, {"$ref": "#/$defs/HelloResponse"}, {"$ref": "#/$defs/SubscribeRequest"},
        {"$ref": "#/$defs/TopicsResponse"}, {"$ref": "#/$defs/CallRequest"}, {"$ref": "#/$defs/CallResponse"},
        {"$ref": "#/$defs/StatusEvent"}, {"$ref": "#/$defs/NowPlayingEvent"},
        {"$ref": "#/$defs/QueueEvent"}, {"$ref": "#/$defs/ShutdownEvent"}, {"$ref": "#/$defs/InvokeRequest"},
        {"$ref": "#/$defs/InvokeResponse"}, {"$ref": "#/$defs/ErrorResponse"}
      ]
    }
  }
}
```
