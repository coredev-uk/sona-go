# Add-ons: protocol v2

An add-on is a program that runs beside Sona, hears what plays and may ask Sona to do small things.

- It talks to Sona over **protocol v2**: one JSON object per line, in both directions, over the add-on's standard input and output.
- It can be written in any language. `internal/cli/testdata/addon-client.sh` (shell) and `addon-client.py` (Python) are two complete clients; Sona's tests run both.
- Complete add-ons to read, copy and run are in [coredev-uk/sona-addons](https://github.com/coredev-uk/sona-addons) (MIT): a now-playing webhook, a Home Assistant bridge, a song.link action and a Python client, all for protocol v2.
- The Go side is `internal/cli` (`addon.go`, `addon_session.go`). The plan is `docs/maintainers/proposals/addons-and-remaining-work.md`.
- Status: the protocol, manifests, the supervisor that starts add-ons, UI contributions, Settings → Add-ons, `sona addons dev` and [WebAssembly add-ons](#webassembly-add-ons) are implemented. `sona addons dev` runs a native add-on on Linux only for now, and a WebAssembly one on every system.

> **Permissions are advisory until the OS sandbox (#304) lands.** A native add-on is a program that runs with your rights. It can read what you can read, including files and the keyring, and it can reach the network. Permissions limit what Sona's protocol hands it; they do not limit the process. Run only add-ons you trust, and say so wherever one is offered for approval. A [WebAssembly add-on](#webassembly-add-ons) is the exception: it runs inside Sona and has no files or network of its own, whatever it asks for.

## Contents

- [Concepts](#concepts)
- [Transport and lifecycle](#transport-and-lifecycle)
- [Messages](#messages)
- [Events](#events)
- [Permissions](#permissions)
- [Limits](#limits)
- [Errors](#errors)
- [Privacy: private mode and previews](#privacy-private-mode-and-previews)
- [Manifest and supervisor](#manifest-and-supervisor)
- [WebAssembly add-ons](#webassembly-add-ons)
- [Bundled add-ons](#bundled-add-ons)
- [Security notes](#security-notes)
- [Schema](#schema)

## Concepts

| Term | Meaning |
| --- | --- |
| Frame | One UTF-8 JSON object ended by `\n`. |
| Request | A frame with an `id` (an integer from 1) and a `method`. Its answer has the same `id` and a `result` or an `error`. |
| Event | A frame with an `event` and `data` and no `id`. Nobody answers it. |
| Topic | A stream of events you subscribe to: `status`, `now-playing`, `queue` or `settings`. |
| Call | A request that runs one of the `sona` CLI's v1 commands for the add-on. |
| Invoke | The one request Sona sends: the user chose one of the add-on's contributions. |
| Permission | What the user grants an add-on. Checked on every call and every event. |

- IDs of the two directions are separate: Sona's `invoke` requests count on their own.
- **Unknown fields are ignored** in what Sona sends. A minor version may add fields, never rename, remove or change one. An add-on must do the same.
- **Only public links.** In events and answers, `artwork` is an https image on Apple's artwork CDN (`*.mzstatic.com`) or empty. A search result's `url` is a `music.apple.com` link or empty. An add-on never sees an Apple API host, a token, audio or video.

## Transport and lifecycle

### The channel

- **It is inherited, not found.** The supervisor starts the add-on with the channel as its standard input and output. The add-on's log goes to its standard error.
- There is no socket to connect to and no name to guess.
- The D-Bus bus and the command socket that carry the `sona` CLI's v1 commands refuse every v2 method (`hello` included). The add-on channel refuses v1.
- **One frame per line**, at most **1 MiB** (the limit is in the `hello` answer). A longer line ends the session.

### The session

1. The add-on sends [`hello`](#hello) as its first frame. Anything else first (a v1 command, a `call`, text that isn't JSON) is answered once with `hello_required`, and the channel is closed.
2. It [subscribes](#subscribe-and-unsubscribe) to topics and receives [events](#events).
3. It sends [`call`](#call)s. Sona [`invoke`](#invoke)s it.
4. When Sona stops the add-on, it sends the [`shutdown`](#events) event, then closes the channel after a short grace.

### Version negotiation

- `hello` carries the range of protocol versions the add-on speaks.
- Sona answers with the **highest version both speak**.
- With no common version, Sona refuses with `version_unsupported`.
- Sona currently speaks `2.0.0` only.

## Messages

All schemas are one [JSON Schema](https://json-schema.org/) document (draft 2020-12; the `$defs` are what the examples cite), in [Schema](#schema). Sona's tests validate every example in this page, and every frame the session sends, against it.

| Message | Direction | Permission | Payload | Notes |
| --- | --- | --- | --- | --- |
| [hello](#hello) | add-on → Sona | none | `id`, `protocol` range, `permissions` | First frame. Answer: version, granted permissions, limits. |
| [subscribe](#subscribe-and-unsubscribe) | add-on → Sona | the topic's | `topics` | Answer: the topics now subscribed to. |
| [unsubscribe](#subscribe-and-unsubscribe) | add-on → Sona | none | `topics` | Answer: the topics now subscribed to. |
| [call](#call) | add-on → Sona | the command's | `command`, `args`, `kind`, `limit` | Result is the command's JSON. |
| [invoke](#invoke) | Sona → add-on | none | `action`, `target`, `deadlineMs` | Answer: optional `toast` and `url`. |

### `hello`

The first frame of every session.

- `protocol` is the range of versions the add-on speaks: `major.minor` or `major.minor.patch`; the patch is ignored.
- An add-on whose minimum is above Sona's highest, or whose maximum is below Sona's lowest, gets `version_unsupported`.
- `id` is the add-on's ID. It must be the one the supervisor started it as.
- A `hello` naming another add-on, or asking for a permission this protocol doesn't have, is refused with `bad_hello`.
- A second `hello` is answered `already_hello`.

```json example HelloRequest
{"id":1,"method":"hello","params":{"id":"now-playing-webhook","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":["playback.read","playback.control"]}}
```

The answer lists the asked-for permissions that are granted now, and the session's limits.

```json example HelloResponse
{"id":1,"result":{"id":"now-playing-webhook","protocol":"2.0.0","permissions":["playback.read"],"limits":{"maxFrame":1048576,"calls":8,"invokes":4,"subscriptions":8,"args":32,"argBytes":4096,"queue":256}}}
```

### `subscribe` and `unsubscribe`

- Both take `topics` and answer with the topics subscribed to afterwards.
- A subscription needs the topic's permission (`status` and `now-playing`: `playback.read`; `queue`: `queue.read`; `settings`: none, its data is the add-on's own).
- At most 8 topics may be subscribed to at once.
- Subscribing delivers the topic's latest value at once, then every change.
- Subscribing again to a topic is harmless.

```json example SubscribeRequest
{"id":2,"method":"subscribe","params":{"topics":["status","now-playing"]}}
```

```json example TopicsResponse
{"id":2,"result":{"topics":["status","now-playing"]}}
```

```json example UnsubscribeRequest
{"id":3,"method":"unsubscribe","params":{"topics":["status"]}}
```

### `secret`

Returns what you saved in one of the add-on's own secret rows. The add-on names the row's `key` (its ID); Sona looks it up under this add-on's ID in the keyring, so an add-on can read no secret but its own. It needs no permission. It fails with `not_found` while the row is empty and with `bad_arguments` for a key that is not a row ID; it counts against the call rate.

```json example SecretRequest
{"id":6,"method":"secret","params":{"key":"token"}}
```

```json example SecretResponse
{"id":6,"result":{"value":"s3cr3t-token"}}
```

### `call`

Runs one of the `sona` CLI's v1 commands (`site/src/content/docs/reference/cli.md`) for the add-on, as the running Sona answers it.

- `params` is the v1 request: `command`, `args`, for `search` `kind` and `limit`, `shuffle` for `play <link> --shuffle` and `force` for `prev --force`.
- Only the commands with a permission in [Permissions](#permissions) are accepted, and not the streams (`--follow`).
- The commands that act on the playing song (`lyrics`, `like`, `dislike`, and `library` but its lists) fail with `nothing_playing` for a hidden listen.
- `open` keeps the CLI's rules: only Apple Music links Sona can open (`bad_link` otherwise). So does `play <link>`.
- `queue jump N` counts places in the queue the add-on sees: hidden listens left out, and the window around the playing song (`Queue`).

| Result | Commands |
| --- | --- |
| `Status` | `status`, `play`, `pause`, `toggle`, `next`, `prev`, `seek`, `shuffle`, `repeat`, `crossfade`, `queue jump` |
| `Queue` | `queue`, `queue clear` |
| `SearchResults` | `search` |
| `Lyrics` | `lyrics` |
| `Like` | `like`, `dislike`, `library` |
| `LibraryItems` | `library playlists`, `library albums` |
| `Volume` | `volume` |
| `Private` | `private` |
| `Opened` | `open` |

The result types are in [Schema](#schema).

```json example CallRequest
{"id":4,"method":"call","params":{"command":"search","args":["abbey road"],"kind":"album","limit":5}}
```

```json example SearchResponse
{"id":4,"result":{"term":"abbey road","results":[{"kind":"album","id":"1441164426","name":"Abbey Road","subtitle":"The Beatles","explicit":false,"url":"https://music.apple.com/gb/album/abbey-road/1441164426"}]}}
```

```json example StatusResponse
{"id":5,"result":{"running":true,"state":"paused","track":null,"position":0,"volume":80,"shuffle":false,"repeat":"off","private":false,"preview":false,"error":"","liked":false,"disliked":false,"inLibrary":false,"crossfade":false}}
```

### `invoke`

The one request Sona sends. When the user chooses one of the add-on's contributions (a palette command, an item action), Sona sends `invoke`.

- `action` is the contribution's ID (`addon.<id>.<name>`).
- `target` is what it was chosen on, or `null`.
- `deadlineMs` is the time the add-on has to answer. **Sona sets the deadline**: 10 seconds by default, never more than 30. A late answer is dropped.
- At most 4 invokes may wait for an answer at once; more fail with `busy`.

```json example InvokeRequest
{"id":1,"method":"invoke","params":{"action":"addon.now-playing-webhook.send","target":{"kind":"album","id":"1441164426","name":"Abbey Road"},"deadlineMs":10000}}
```

The add-on answers with the same `id`. Both fields of the result are plain text and optional:

| Field | Rule | Sona's use |
| --- | --- | --- |
| `toast` | At most 500 bytes. | Shown as a toast under the add-on's name. |
| `url` | `http` or `https`, ASCII, no user name or password, at most 2048 bytes. | Offered as a button. **It opens only after a click.** |

Anything else makes the invoke fail.

```json example InvokeResponse
{"id":1,"result":{"toast":"Sent","url":"https://example.com/played"}}
```

An add-on that cannot do it answers with an `error`. Sona passes only its `code` on.

## Events

| Event | Data | When |
| --- | --- | --- |
| `status` | `Status` | Any change of playback, volume, shuffle, repeat or crossfade, of the playing song's favourite, Suggest Less or library state, and a position that jumps (a seek). A position that only moves on is not an event; compute it from `position` and the state. |
| `now-playing` | `NowPlaying` | The song that plays changes, and when nothing plays. |
| `queue` | `Queue` | The queue changes. At most 1000 songs, a window around the playing one. |
| `settings` | [`SettingsValues`](#settingsvalues) | The value of each of the add-on's non-secret settings rows, now and whenever one changes. A row nobody set has its default. |
| `shutdown` | none | Sent to every session, subscribed or not: Sona is stopping the add-on. |

**A slow reader never slows Sona.**

- Each session keeps only the newest event of each topic. An add-on that reads slowly sees the latest state, not every step.
- An add-on that lets more than 256 frames wait unread is ended.

```json example StatusEvent
{"event":"status","data":{"running":true,"state":"playing","track":{"id":"1441164430","title":"Come Together","artist":"The Beatles","album":"Abbey Road","duration":259.0,"explicit":false,"artwork":"https://is1-ssl.mzstatic.com/image/thumb/Music/600x600bb.jpg"},"position":12.5,"volume":80,"shuffle":false,"repeat":"off","private":false,"preview":false,"error":"","liked":true,"disliked":false,"inLibrary":true,"crossfade":false}}
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

```json example SettingsEvent
{"event":"settings","data":{"values":{"url":"https://example.com/hook","post":"true","format":"json"}}}
```

```json example ShutdownEvent
{"event":"shutdown"}
```

## Permissions

An add-on names the permissions it wants in `hello`. The user grants some of them.

- **Every call and every event checks the grant again.** Revoking a permission refuses the very next call and stops the events at the next change.
- The `hello` answer says only what held at that moment.

| Permission | What it allows |
| --- | --- |
| `playback.read` | Topics `status` and `now-playing`; the `status` call. The status carries whether the playing song is a favourite, rated Suggest Less or in the library. |
| `playback.control` | `play` (a link too), `pause`, `toggle`, `next`, `prev`, `seek`, `shuffle`, `repeat`, `crossfade`. |
| `queue.read` | Topic `queue`; the `queue` call. |
| `queue.control` | `queue jump` and `queue clear`. |
| `lyrics.read` | The `lyrics` call. |
| `search` | The `search` call (Apple's catalogue; rate limited). |
| `open` | The `open` call: Apple Music links only, as the CLI's `open`. |
| `volume` | The `volume` call. |
| `like` | The `like` and `dislike` calls (rate limited). |
| `library` | The `library` call: the playing song added or removed, the library's playlists and albums listed (rate limited). |
| `private.read` | The `private` call, to read whether private listening is on. |
| `network` | Reach the internet. No call needs it: the OS sandbox gives the add-on's process a network when it holds this permission and none otherwise ([How Sona runs an add-on](#how-sona-runs-an-add-on)). |

- Every v1 request maps to exactly one permission: its command's, but `queue.control` for `queue`'s verbs. `network` is no call's. An add-on may send only the commands in this table; any other (`help`, `completion`, a typo) is `unknown_command`.
- **An add-on can never switch private listening off or on.** `private` takes no argument but `status`, whatever is granted.

### Granting

- Enabling an add-on shows the permissions its manifest asks for. Once you approve them, Sona saves them as granted.
- The grant is yours and stays on this computer. A settings export leaves it out, and a settings import never sets it.
- An update that **asks for another permission** does not run until you approve again.
- Revoking a permission refuses the add-on's very next call, and the add-on needs approval before it runs again.
- Permissions are advisory until the OS sandbox (#304), so enabling an add-on is trusting it as you trust any program you run.

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
| `like`, `library`, `open` | 1 per second, burst 3 | `rate_limited` |
| Toasts from invocations | 1 per second per add-on | The later answer is dropped |
| Contributions | 8 actions, 8 commands, 16 rows, 32 in all | The manifest is refused |
| Toast text | 500 bytes | The answer fails |
| Link in an answer | 2048 bytes | The answer fails |

## Errors

An error frame carries a code and **a fixed message** for that code.

- It never carries the text of the error that caused it: that text could hold a URL, a path or a token.
- Switch on `code`. The message is for people.

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

## Privacy: private mode and previews

The user's private listening is the user's alone. For a **private** listen or a **30-second preview**, an add-on gets:

- no event: the `now-playing` topic says nothing is playing, the `status` topic says `idle`, the `queue` topic is empty;
- reads that answer as if nothing played: `status` is idle, `queue` is empty, `lyrics` and `like` fail with `nothing_playing`;
- the same after private listening is switched **off** again: a listen that was private at any point since it started stays hidden until that listen ends.

Turning private mode on in the middle of a listen is not recalled: what was already sent (its start) was already sent. Sona judges this once, in one place, with the time the listen started.

What an add-on never sees:

- **`private: true` or `preview: true`.** Both are always `false` in a status: a listen it may see is public.
- **Error text.** A status's `error` is always `""`. The text of a playback failure can hold a URL, a path or the ID of a hidden listen's song. The `state` still says `error`.
- **Hidden queue slots.** The queue leaves out slots of hidden listens and numbers the rest from 0.
- **A preview that looks public.** Once a listen has been a preview, it stays one.

## Manifest and supervisor

### Installing an add-on

- An add-on is a folder in the `addons` directory of Sona's config directory (`~/.config/sona/addons` on Linux).
- Sona creates the directory with mode 0700: an add-on is code Sona runs, so only you add one.
- The folder's name is the add-on's ID. It holds the manifest, `addon.json`, and the program the manifest names.

```json example Manifest
{"id":"now-playing-webhook","name":"Now playing webhook","version":"1.2.0","description":"Posts the song that plays to a web address.","command":"bin/webhook","permissions":["playback.read"]}
```

### Manifest fields

| Field | Rules |
| --- | --- |
| `id` | Required. Lowercase letters, digits and hyphens, starting with a letter, 2 to 40 characters, no dots. It equals the folder's name (compared without regard to case). `lastfm`, `listenbrainz`, `discord` and anything starting with `sona` are reserved for Sona's own bundled add-ons and refused in your folder. |
| `name` | Required. 1 to 60 characters of plain text: no control, bidirectional or other format characters, no space at either end. Shown wherever the add-on contributes something. |
| `version` | Required. Semver (`1.2.0`). |
| `description` | Optional. At most 300 characters, plain text like `name`. |
| `command` | Required. The program to run, a path inside the folder: relative, `/` separators, no `..`, no `.` or empty element, no drive or backslash, at most 200 bytes. For `runtime` `"wasm"`, a `.wasm` file. |
| `runtime` | Optional. `"native"` (the default): `command` is an executable run as a process of its own. `"wasm"`: `command` is a WebAssembly module Sona runs inside its own process; see [WebAssembly add-ons](#webassembly-add-ons). A WebAssembly add-on may not ask for `network`. |
| `permissions` | Required, may be empty. The [permissions](#permissions) the add-on asks for, each at most once. |
| `contributions` | Optional. What the add-on adds to Sona's menus, palette and Settings: see [Contributions](#contributions). |
| `strings` | Optional. The labels of the contributions by locale: see [Contributions](#contributions). |

- The manifest is at most 64 KiB, and **a field this version doesn't know refuses it**.
- At most 100 folders are read.
- Why an add-on is refused is one of these codes, shown in Settings: `unreadable`, `manifest_too_large`, `manifest_invalid`, `bad_id`, `reserved_id`, `id_mismatch`, `bad_name`, `bad_version`, `bad_command`, `bad_runtime`, `bad_permission`, `bad_contribution`, `bad_strings`, `no_executable`, `executable_escapes`, `unsafe_owner`, `too_many_addons`.

### Contributions

A manifest may add to three of Sona's own surfaces. **Sona draws all of it**: an add-on brings text, never code or pictures into Sona's window. Choosing an action or a command sends the add-on an [`invoke`](#invoke); its answer is shown as a toast.

```json example ManifestContributions
{"id":"now-playing-webhook","name":"Now playing webhook","version":"1.2.0","command":"bin/webhook","permissions":["playback.read"],"contributions":{"actions":[{"id":"send","label":"Send to webhook","kinds":["song","album"],"icon":"send"}],"commands":[{"id":"refresh","label":"Refresh the webhook","keywords":"post address"}],"settings":[{"id":"url","kind":"text","label":"Web address","description":"Where the song is posted."},{"id":"post","kind":"toggle","label":"Post songs","default":"true"},{"id":"format","kind":"choice","label":"Format","options":[{"value":"json","label":"JSON"},{"value":"text","label":"Plain text"}],"default":"json"},{"id":"token","kind":"secret","label":"Access token"}]},"strings":{"de":{"action.send":"An Webhook senden","setting.url":"Webadresse"}}}
```

| Surface | What it is | Where Sona shows it |
| --- | --- | --- |
| **Actions** | Entries for a song, album, artist or playlist (`kinds`). | The item's menu, in a section of its own at the end. |
| **Commands** | Entries with no item. | The command palette, under "Add-ons". |
| **Settings rows** | A `toggle` (`"true"` or `"false"`), `text`, a `choice` of 2 to 16 `options`, or a `secret`. | Settings → Add-ons, under the add-on's own heading. Never on a sign-in or account page. |

**IDs and keys**

- An action or command has a local `id`: lowercase letters, digits and hyphens, at most 40.
- Its full ID is `addon.<add-on id>.<id>`. That is what `invoke` carries.
- **An add-on can never use an ID outside `addon.<its id>.*`.**
- A command has **no keys**. It is not in Settings → Shortcuts and an add-on can't take a key from Sona.
- `icon` names one of Sona's own icons. An unknown name gets a plain one. There are no pictures.

**Settings rows**

- The add-on reads the values from the `settings` topic ([Events](#events)).
- It reads a secret with the [`secret`](#secret) method.
- **A secret is kept in the system keyring under `addon.<id>`, never in `settings.json`.** Settings shows it as "Saved" and never shows it again.
- The other values are part of the add-on's local settings. An export leaves them out.

**Text and limits**

- Labels are plain text of 1 to 60 characters. Descriptions are plain text up to 200.
- Labels hold no control, bidirectional or other format characters.
- Sona shows the add-on's name beside every contribution.
- At most 8 actions, 8 commands and 16 rows, 32 in all.

**`strings`** gives the labels in other languages, by locale: `{"de": {"action.send": "…"}}`.

| Key | Names |
| --- | --- |
| `action.<id>` | An action's label. |
| `command.<id>` | A command's label. |
| `command.<id>.keywords` | A command's search words. |
| `setting.<id>` | A row's label. |
| `setting.<id>.description` | A row's description. |
| `option.<row id>.<value>` | A choice's label. |

- A key that names nothing refuses the manifest.
- Sona picks the exact locale, then the language, then the manifest's own text.
- These strings are outside Sona's own translation catalogues.

**What Sona does with an invocation**

- Sona calls the add-on off the UI thread. The same entry chosen again while it runs does nothing.
- `target` is the item the action was chosen on, `{"kind": "album", "id": "…", "name": "…"}`. `id` is the Apple Music catalogue ID, or the library ID of a song only in the library. It is `null` for a command.
- The answer's `toast` is shown for a few seconds under the add-on's name, **at most one a second per add-on**. Text is cut to 500 bytes and stripped of control characters.
- A toast has **no buttons that run code**. A link in the answer is offered as one button, "Open {host}".
- **A link opens only after you click that button.** It must be `http` or `https`, ASCII, with no user name or password, and at most 2048 bytes. `file:`, `search-ms:` and every other scheme is refused, and the answer fails.

### Safety rules for the files

Sona refuses an add-on when:

- the folder is a link, or the program, once every link in its path is followed, is not inside the folder (`executable_escapes`);
- the program is not a regular file, or on Linux and macOS not executable (`no_executable`; a WebAssembly module is data and needs no execute bit);
- on Linux and macOS, the folder, the manifest, the program or any folder on the way to it belongs to another user, or is writable by its group or by everyone (`unsafe_owner`). Windows keeps a profile's folders to their user by ACLs, which Sona doesn't read.

Sona runs the program by the absolute path it checked: never through `PATH` or the working directory. A WebAssembly module is not run by path: Sona reads the open file once and compiles those bytes.

### How Sona runs an add-on

These rules are for native add-ons; a [WebAssembly add-on](#webassembly-add-ons) follows the same start, restart and log rules and runs as that section says.

- **After the first paint**, never during start-up. A hello must arrive within 10 seconds, or the add-on is killed.
- **Its environment** is an allowlist:
  - `HOME`, `PATH`, `LANG`, `LANGUAGE`, `LC_*`, `TZ`, `TMPDIR`;
  - on Windows, what a process needs to start: `SystemRoot`, `SystemDrive`, `TEMP`, `TMP`, `USERPROFILE`, `PATHEXT`, `ComSpec`;
  - `SONA_ADDON_ID` and `SONA_ADDON_PROTOCOL=2`.
  - A token, a key, a bus address or anything else in Sona's environment never reaches it. Its working directory is its folder.
- **Its processes end with Sona's.** An add-on leads a process group of its own (a job object on Windows). When Sona quits, it sends the `shutdown` event, closes the add-on's input, gives it a second, asks it to terminate, gives it a second more, then kills the whole group and reaps it.
- On Linux the kernel also kills the add-on if Sona dies without quitting. macOS has no such signal: when Sona is killed the add-on's input closes, and **an add-on must exit when its input ends**. Do not start children that leave the group.
- **It is restarted when it stops on its own**, after 1, 2, 5, 15 and then 30 seconds. After 5 stops in a row, with no run of a minute between, it is switched off and Sona tells you.
- **Its log** is `addons/<id>.log` in Sona's state directory, beside `sona.log`:
  - what it writes to standard error, with the time, control characters replaced by spaces and lines cut at 4 KiB;
  - Sona's own notes, marked `sona:`;
  - mode 0600, rotated at 1 MiB with one older copy kept.
  - The bug-report bundle lists add-ons (ID, version, on or off) and never their logs.

### Settings → Add-ons

- Lists each add-on under its own heading, refused ones too, with the reason.
- **The switch.** Turning an add-on on opens a panel under the row. It names each permission and what it allows. Nothing is saved until you press "Turn on".
- **Permissions.** Each one is shown as approved or not, with "Revoke" on the approved ones.
- **Rows.** The add-on's settings rows, saved at once (a text field with "Save").
- **Log.** "Show log" reads the last 200 lines. They are plain text.
- **Folder.** "Open add-ons folder" creates the folder (mode 0700) and opens it. "Look again" reads the folder again, and so does coming back to the page.
- The page says first that a native add-on runs with your rights and that permissions are advisory until the OS sandbox (#304). A WebAssembly add-on says in its own panel that it runs inside Sona with no files or network, and is marked "WebAssembly" under its name.

### Developing an add-on

[coredev-uk/sona-addons](https://github.com/coredev-uk/sona-addons) has samples that run under this command, a small Go client of the protocol and a fake Sona for tests.

`sona addons dev <folder>` runs an add-on from a folder **without installing it**, and prints its log until you press Ctrl+C. It needs Sona running. A native add-on needs Linux for now; a [WebAssembly add-on](#webassembly-add-ons) runs on every system Sona does.

| Rule | How it is kept |
| --- | --- |
| The folder is yours. | Checked as an installed folder is: yours, not writable by group or others, not a link. |
| It is not an installed add-on. | The ID may not belong to any folder in the add-ons directory, refused ones included, or to a running add-on. A folder inside the add-ons directory is installed. |
| **You confirm it in Sona's window.** | A dialog names the add-on, the folder and the permissions. Sona brings its window up. One question at a time. No answer within 60 seconds is "no". |
| It is not reachable from an add-on. | The command is not on the add-on channel's list: an add-on that sends `addons` gets `unknown_command`. |
| Grants are not saved. | The manifest's permissions hold for the run, in memory. The settings file and the keyring are not touched. |
| It gets none of your data. | Rows have their default values. `secret` finds nothing, even if the ID once was an installed add-on's. |
| The file you confirmed is the file that runs. | The folder is read again after your answer, and a change to the program, the ID, the version or the permissions is refused. Native: Sona opens the program once, checks the **open file** (a regular file of yours, no group or other write, inside the folder) and runs that descriptor through `/proc/self/fd`. WebAssembly: Sona opens the module once, checks the open file the same way, reads it and runs **those bytes**. In both a path swapped meanwhile changes nothing. |
| It ends when you leave. | Ctrl+C stops it. A terminal that dies without saying so is noticed after 15 seconds without a poll. It also ends when Sona quits. |
| The log is safe for a terminal. | Sona's log already replaces control characters. On a terminal the client also removes escape sequences (CSI, OSC, DCS, and their 8-bit forms), bidirectional characters and the other control characters. |

- A native add-on runs with your rights and with the descriptor of its own program open as file 3. A script sees `/proc/self/fd/3` as `$0`.
- A WebAssembly add-on runs in Sona like an installed one, with no data folder: it gets none of your data.
- Its actions and commands are offered while it runs. It has no settings rows.
- The log is also written to `addons/<id>.dev.log`, beside the log of installed add-ons.
- Running a native add-on on macOS or Windows fails with "unsupported": they can't yet run the file that was checked, and the question is not asked. A WebAssembly add-on has no such need: nothing is run from a path, so the Linux-only rule is a rule of the native runtime, not of the command.

**How the command talks to Sona**

- `addons dev <folder>` starts the run. Sona answers once you confirm.
- The client then asks for new lines, about three times a second, with a token only it holds.
- Ctrl+C sends one last request that stops the run.
- These requests are the client's own. Only `dev` is a command you can type.

## WebAssembly add-ons

An add-on with `"runtime": "wasm"` is a [WebAssembly](https://webassembly.org/) module that Sona runs **inside its own process**, on [wazero](https://wazero.io) (a WebAssembly runtime written in Go, with no cgo), instead of a program of its own. It speaks the same protocol v2 over the module's standard input and output, with the same manifest, contributions, permissions and [limits](#limits); the session code is the one native add-ons use. Only how it runs differs.

```json example ManifestWasm
{"id":"copy-songlink","name":"Copy song.link","version":"1.0.0","command":"copy-songlink.wasm","runtime":"wasm","permissions":[],"contributions":{"actions":[{"id":"copy","label":"Copy song.link","kinds":["song","album"]}]}}
```

### What it is and what it can't do

- **A WASI command.** The module is built for WASI preview 1 (`wasip1`) and exports `_start`. Its standard input and output are the channel, its standard error is its [log](#how-sona-runs-an-add-on). A module that needs an import Sona doesn't give (WASI preview 2, threads, a host function of another runtime) fails to start, and its log says why.
- **It has no ambient authority.** It can use only what Sona hands it:
  - its standard streams, the clock, random bytes, its arguments (the first is `<id>.wasm`) and its environment: `LANG`, `LANGUAGE`, `LC_*`, `TZ`, `SONA_ADDON_ID` and `SONA_ADDON_PROTOCOL=2`. `HOME`, `PATH` and `TMPDIR` are left out: they name places that don't exist for it;
  - no folder of yours. It sees no file at all, so `open("/etc/passwd")`, `..` and a host path all fail. A module with a data folder sees only that folder, as `/data`, in `SONA_ADDON_DATA`; links in it are refused, and the module can't make one (see [Why it needs no OS sandbox](#why-it-needs-no-os-sandbox)).
- **It has no network.** WASI preview 1 has no sockets, so there is nothing for a module to import. That is why a manifest with `"runtime": "wasm"` that asks for `network` is refused (`bad_permission`): it would promise what the module can never use. A module can still show you a link: the `url` of an [`invoke`](#invoke) answer, which opens only after you click it.
- **No processes, no threads.** A module is one thread, and a read of its standard input blocks that thread: while any part of the module waits for the next frame, **nothing else in it runs**, goroutines of a Go module included. A Go client that reads standard input in a goroutine of its own hangs after its first answer, because the reader is back in its blocking read before the goroutine that should send the next request has run. Write the add-on as one loop that reads a frame, acts and answers, and make a request by writing it and reading frames until its answer comes. The same code runs natively. Don't wait on a timer while waiting for input.

### Why it needs no OS sandbox

The OS sandbox for native add-ons (#304) exists because the OS runs a program with your rights and something must take them away. A module is not that. It is bytes a runtime compiles, and the only way out of it is through functions the host defines:

- the module can reach a file only by a WASI call on a folder Sona opened for it, and the file functions Sona gives it are `confinedFS` (`internal/addons/wasmfs.go`): wazero's own directory mount follows a link wherever it points and lets a module create one, so Sona refuses every path through a link and every `symlink`;
- WASI has no call for a socket, a process or the environment of the host, so nothing is there to be denied;
- WASI's `..` and absolute paths are refused by the runtime before they reach the folder.

What this does not do is isolate Sona from the module's **resource use**: the module runs in Sona's address space.

### What is capped, and what is not

| | Cap |
| --- | --- |
| Memory | **64 MiB** of linear memory per module. A growth past it fails, as on a machine out of memory, and a module usually ends there. 64 MiB is room for a Go module (its runtime wants about 20 MiB), while a leak or a hostile allocation costs Sona no more than that. |
| CPU | **A quarter of a core, for long**, with a burst of five seconds. The module's own running time fills a bucket that refills at that rate; the time it waits for its input, or sleeps, costs nothing. A module that exhausts it is stopped and its log says `stopped: used more CPU than a wasm add-on may`. A loop that never ends is stopped after about seven seconds. |
| Frames, calls, subscriptions, answers | The same [limits](#limits) as any add-on. |

Not capped:

- **Memory in total.** Each module's 64 MiB is part of Sona's heap (a Go slice) and counts in Sona's memory, and there is no limit across add-ons.
- **Compiling.** The first start of a module compiles it, on at most two cores, for a second or two (a few seconds for a large module), off the UI thread. Later starts load the [cache](#compiled-code).
- **A flaw in the runtime.** A bug in wazero, or in the code Sona runs it with, is a bug in Sona's own process. That is the price of running inside it; it is why the dependency was reviewed (`docs/security-review.md`).

### Compiled code

- A module is compiled once and the result is cached in `addons-wasm` in Sona's cache directory (`~/.cache/sona` on Linux), keyed by the hash of the module (and wazero's version, the OS and the architecture). A module that hasn't changed isn't compiled again.
- It is machine code that Sona loads, so the folder is Sona's own: mode 0700, outside every add-on's reach. A folder or entry that isn't yours alone, or that others can write, is not trusted: Sona deletes the whole cache and starts again. It is kept to 256 MiB the same way.
- It is a cache. Deleting it costs one compile. If it can't be used, Sona keeps compiled code in memory and the add-on's log says so.

### Running

- **Start and restart** are a native add-on's: after the first paint, a hello within 10 seconds, restarts after 1, 2, 5, 15 and 30 seconds, and switched off after 5 stops in a row with no run of a minute between. A module that is stopped for its CPU or that runs out of memory counts as stopped on its own.
- **The module file** is read when the add-on starts, and checked on the open file: a regular file of yours that no one else can write, inside the folder (links followed, as for any command), at most 32 MiB, starting `\0asm`. It needs no execute bit. What is read is what is compiled.
- **Stopping.** When Sona stops the add-on it sends the `shutdown` event and ends the module's input, as for a native one; a module should leave when either comes. It is given the same second, then killed. There is no terminate step. A kill stops a module at its next loop or call and frees a read or write it waits in.
- **Never on the UI thread.** Each module runs on its own goroutine, compiling included, and talks to Sona through pipes; the UI never waits for it.
- The add-on's log says `started (WebAssembly in Sona, version …)` where a native add-on's says its process ID.

### Building one

Anything that makes a WASI preview 1 command works. The same Go source that uses only standard input and output builds natively and as a module.

```sh
# Go 1.21 or later: the standard toolchain. About 2 MiB for a small add-on.
GOOS=wasip1 GOARCH=wasm go build -o copy-songlink.wasm .

# TinyGo: far smaller modules.
tinygo build -target=wasip1 -o copy-songlink.wasm .

# Rust.
rustup target add wasm32-wasip1
cargo build --target wasm32-wasip1 --release   # target/wasm32-wasip1/release/<name>.wasm
```

Put the module and an `addon.json` with `"runtime": "wasm"` and `"command"` naming it in a folder, and run it with [`sona addons dev`](#developing-an-add-on).

## Bundled add-ons

Sona's own Last.fm, ListenBrainz and Discord integrations are add-ons: `lastfm`, `listenbrainz` and `discord`. They move onto this API one at a time (#302); this section says what makes them different from yours.

- **Registered in code.** Their manifests are embedded in Sona (`internal/addons/bundled`) and are never read from your add-ons folder. A folder with one of these IDs is refused by the scanner, left out of Settings → Add-ons and never run; the one Sona lists is the registered one.
- **The program is Sona itself.** The command of each is Sona's own executable with `--addon <id>`, so the release's signature covers it and there is no second key. `sona --addon <id>` is for the supervisor: started by hand it waits for a hello that nobody sends. It starts nothing else of Sona: no window, no settings, no keyring.
- **The same supervisor, protocol and checks.** They are started after the first paint, restarted with the same backoff, bounded by the same limits, and hold only the permissions their manifest names. Like yours they see no private listen and no preview.
- **On by default, running when there is something to do.** Settings → Add-ons lists them as Sona's own, each with a switch and a log. There is nothing to approve and no permission to revoke: they are part of the signed release. Each is a process of Sona's size, so one runs only while its integration is in use: Last.fm and ListenBrainz while an account is connected (also paused, as listens already queued are still delivered), Discord while its status is on. Until then the page says it is on and starts when Sona has something for it to do. Settings → Connections stays where you connect an account and switch each one on or off.
- **The folder they run from** is the folder Sona is installed in.
- **Status.** The registry, the entry point and the supervision are in. Last.fm, ListenBrainz and Discord still do their work inside Sona; their add-ons say hello and wait until each one moves onto the API.

## Security notes

- **Permissions are advisory until #304 for native add-ons.** See the callout at the top.
- **A WebAssembly add-on has no ambient authority.** No file but its data folder, no network, no process; links in its folder are refused. Its CPU and memory are capped, and it runs inside Sona's process, so a flaw in the runtime is a flaw in Sona.
- **No ambient secrets.** The environment is an allowlist; tokens, keys and bus addresses never reach the add-on.
- **No guessable channel.** It is inherited, and the bus and command socket refuse v2.
- **No private data.** Hidden listens are invisible, `private` and `preview` are always `false`, and error text is always empty.
- **Fixed error messages.** An error never carries the cause's text.
- **Public links only.** Artwork and search URLs are checked; no API host, token, audio or video.
- **Private mode is the user's.** An add-on can never switch it.
- **Files are checked.** Links, owners, write bits and escaping paths refuse the add-on.
- **Everything is bounded.** Frames, calls, invokes, subscriptions and the unread queue have [limits](#limits).
- **Sona's own add-ons are code, not folders.** They are registered in the binary, never read from your folder, and a folder that claims their IDs never runs: see [Bundled add-ons](#bundled-add-ons).

## Schema

```json schema
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/coredev-uk/sona-go/docs/addons.md",
  "$defs": {
    "Permission": {"enum": ["playback.read", "playback.control", "queue.read", "queue.control", "lyrics.read", "search", "open", "volume", "like", "library", "private.read", "network"]},
    "WasmPermission": {"enum": ["playback.read", "playback.control", "queue.read", "queue.control", "lyrics.read", "search", "open", "volume", "like", "library", "private.read"]},
    "Topic": {"enum": ["status", "now-playing", "queue", "settings"]},
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
      "required": ["running", "state", "track", "position", "volume", "shuffle", "repeat", "private", "preview", "error", "liked", "disliked", "inLibrary", "crossfade"],
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
        "error": {"const": ""},
        "liked": {"type": "boolean"}, "disliked": {"type": "boolean"}, "inLibrary": {"type": "boolean"},
        "crossfade": {"type": "boolean"}
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
      "type": "object", "additionalProperties": false, "required": ["track", "liked", "disliked", "inLibrary"],
      "properties": {
        "track": {"$ref": "#/$defs/Track"}, "liked": {"type": "boolean"}, "disliked": {"type": "boolean"},
        "inLibrary": {"type": "boolean"}
      }
    },
    "LibraryItems": {
      "type": "object", "additionalProperties": false, "required": ["kind", "total", "items"],
      "properties": {
        "kind": {"enum": ["playlist", "album"]},
        "total": {"type": "integer", "minimum": 0},
        "items": {"type": "array", "maxItems": 500, "items": {
          "type": "object", "additionalProperties": false, "required": ["id", "name", "subtitle", "artwork", "url"],
          "properties": {
            "id": {"type": "string"}, "name": {"type": "string"}, "subtitle": {"type": "string"},
            "artwork": {"$ref": "#/$defs/Artwork"}, "url": {"$ref": "#/$defs/ShareURL"}
          }
        }}
      }
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
            "permissions": {"type": "array", "maxItems": 12, "uniqueItems": true, "items": {"$ref": "#/$defs/Permission"}}
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
            "command": {"enum": ["play", "pause", "toggle", "next", "prev", "seek", "shuffle", "repeat", "crossfade", "status", "queue", "lyrics", "search", "open", "volume", "like", "dislike", "library", "private"]},
            "args": {"type": "array", "maxItems": 32, "items": {"type": "string", "maxLength": 4096}},
            "kind": {"enum": ["song", "album", "artist", "playlist", "station", "music-video", "curator"]},
            "limit": {"type": "integer", "minimum": 1, "maximum": 20},
            "shuffle": {"type": "boolean"},
            "force": {"type": "boolean"}
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
          {"$ref": "#/$defs/Like"}, {"$ref": "#/$defs/LibraryItems"}, {"$ref": "#/$defs/Volume"}, {"$ref": "#/$defs/Private"},
          {"$ref": "#/$defs/Opened"}
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
    "SettingsEvent": {
      "type": "object", "additionalProperties": false, "required": ["event", "data"],
      "properties": {
        "event": {"const": "settings"},
        "data": {
          "type": "object", "additionalProperties": false, "required": ["values"],
          "properties": {"values": {"type": "object"}}
        }
      }
    },
    "SecretRequest": {
      "type": "object", "additionalProperties": false, "required": ["id", "method", "params"],
      "properties": {
        "id": {"type": "integer", "minimum": 1}, "method": {"const": "secret"},
        "params": {
          "type": "object", "additionalProperties": false, "required": ["key"],
          "properties": {"key": {"type": "string", "pattern": "^[a-z][a-z0-9-]{0,39}$"}}
        }
      }
    },
    "SecretResponse": {
      "type": "object", "additionalProperties": false, "required": ["id", "result"],
      "properties": {
        "id": {"type": "integer", "minimum": 1},
        "result": {
          "type": "object", "additionalProperties": false, "required": ["value"],
          "properties": {"value": {"type": "string", "maxLength": 1024}}
        }
      }
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
        "runtime": {"enum": ["native", "wasm"]},
        "permissions": {"type": "array", "maxItems": 12, "uniqueItems": true, "items": {"$ref": "#/$defs/Permission"}},
        "contributions": {
          "type": "object", "additionalProperties": false,
          "properties": {
            "actions": {"type": "array", "maxItems": 8, "items": {
              "type": "object", "additionalProperties": false, "required": ["id", "label", "kinds"],
              "properties": {
                "id": {"$ref": "#/$defs/LocalID"}, "label": {"type": "string", "minLength": 1, "maxLength": 240},
                "kinds": {"type": "array", "minItems": 1, "maxItems": 4, "uniqueItems": true, "items": {"enum": ["song", "album", "artist", "playlist"]}},
                "icon": {"$ref": "#/$defs/LocalID"}
              }
            }},
            "commands": {"type": "array", "maxItems": 8, "items": {
              "type": "object", "additionalProperties": false, "required": ["id", "label"],
              "properties": {
                "id": {"$ref": "#/$defs/LocalID"}, "label": {"type": "string", "minLength": 1, "maxLength": 240},
                "keywords": {"type": "string", "maxLength": 240}, "icon": {"$ref": "#/$defs/LocalID"}
              }
            }},
            "settings": {"type": "array", "maxItems": 16, "items": {
              "type": "object", "additionalProperties": false, "required": ["id", "kind", "label"],
              "properties": {
                "id": {"$ref": "#/$defs/LocalID"}, "kind": {"enum": ["toggle", "text", "choice", "secret"]},
                "label": {"type": "string", "minLength": 1, "maxLength": 240}, "description": {"type": "string", "maxLength": 800},
                "default": {"type": "string", "maxLength": 800},
                "options": {"type": "array", "minItems": 2, "maxItems": 16, "items": {
                  "type": "object", "additionalProperties": false, "required": ["value", "label"],
                  "properties": {"value": {"type": "string", "pattern": "^[A-Za-z0-9._-]{1,40}$"}, "label": {"type": "string", "minLength": 1, "maxLength": 240}}
                }}
              }
            }}
          }
        },
        "strings": {"type": "object"}
      },
      "oneOf": [
        {"properties": {"runtime": {"enum": ["native"]}}},
        {"required": ["runtime"], "properties": {"runtime": {"const": "wasm"}, "command": {"pattern": "\\.wasm$"}, "permissions": {"items": {"$ref": "#/$defs/WasmPermission"}}}}
      ]
    },
    "LocalID": {"type": "string", "pattern": "^[a-z][a-z0-9-]{0,39}$"},
    "Frame": {
      "oneOf": [
        {"$ref": "#/$defs/HelloRequest"}, {"$ref": "#/$defs/HelloResponse"}, {"$ref": "#/$defs/SubscribeRequest"},
        {"$ref": "#/$defs/TopicsResponse"}, {"$ref": "#/$defs/CallRequest"}, {"$ref": "#/$defs/CallResponse"},
        {"$ref": "#/$defs/StatusEvent"}, {"$ref": "#/$defs/NowPlayingEvent"},
        {"$ref": "#/$defs/QueueEvent"}, {"$ref": "#/$defs/ShutdownEvent"}, {"$ref": "#/$defs/SettingsEvent"},
        {"$ref": "#/$defs/SecretRequest"}, {"$ref": "#/$defs/SecretResponse"}, {"$ref": "#/$defs/InvokeRequest"},
        {"$ref": "#/$defs/InvokeResponse"}, {"$ref": "#/$defs/ErrorResponse"}
      ]
    }
  }
}
```
