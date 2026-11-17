# Proposal: Stream Deck control, at parity with CiderDeck

Sona gets a Stream Deck plugin that does what Cider's official one ([CiderDeck](https://github.com/ciderapp/CiderDeck), read at `f1901da`, 23 actions) does where Sona has the feature. The plugin talks to the `sona` command line's protocol; Sona opens no network port for it.

## Decisions

- **No local HTTP or WebSocket server.** CiderDeck talks to Cider's RPC server on `127.0.0.1:10767` with a paired app token. A TCP listener can't tell which local user connects (the command socket checks peer credentials, `internal/app/socket.go`). Any web page can open `ws://127.0.0.1` (WebSockets skip CORS), so it would need pairing, Origin and Host checks and DNS-rebinding defences. It would also be a third protocol beside v1 and add-on v2. Cider needs one because its state lives in a web renderer; Sona's lives in Go behind a transport that already exists.
- **No process per key press.** Running `sona toggle` for each press works, but the dial sends many ticks a second, each start costs a process on Windows (and a console flash unless hidden), and the tiles still need a status stream.
- **Not the plugin on the socket directly.** Linux uses D-Bus, and Node's `net` on Windows speaks named pipes, not the AF_UNIX socket Sona listens on [INFERENCE: libuv's Windows pipes; not tried]. The plugin would reimplement both transports and version handling.
- **Not an add-on.** Sona starts add-ons; the Stream Deck app starts its plugin. Add-ons also can't switch private listening, by design.
- **Instead:** the plugin (`@elgato/streamdeck`, Node) starts one long-lived `sona connect` and speaks JSON lines with it over stdin/stdout, with add-on protocol v2's framing (`call`, `subscribe`, events). `sona connect` forwards to the running Sona over the CLI's transport (D-Bus on Linux, `sona.sock` elsewhere). Whoever can run `sona` already has the CLI's rights, so there is no pairing.
- **Repository:** the plugin lives in its own repository (`coredev-uk/sona-streamdeck`), as CiderDeck does, to keep Node out of this repository's toolchain. The contract is `site/src/content/docs/reference/cli.md` and `internal/cli/schema.go`.

## Parity

| CiderDeck action | Sona |
| --- | --- |
| Play/pause, next, previous (option: always previous) | `toggle`, `next`, `prev [--force]` |
| Volume up and down, dial volume | `volume ±N` |
| Dial seek | `seek <s\|+N\|-N>` |
| Shuffle, three-state repeat | `shuffle`, `repeat [… \|cycle]` |
| Favourite, Dislike, Add to Library, with live state | `like`, `dislike`, `library [add\|remove\|toggle]`; the status carries `liked`, `disliked`, `inLibrary` |
| Crossfade | `crossfade`; the status carries `crossfade` |
| Clear queue, queue coverflow and jump | `queue clear`, `queue jump N`, `queue --json` |
| Pinned playlist, album or station, shuffled or not, picked from the library | `play <link> [--shuffle]`, `library playlists\|albums`, `search --type station` |
| Song display, album art, art grid, logo | The plugin, from `status.track` |
| Stream Deck + dial and action wheel | The plugin, from the commands above |
| Atmos | Not yet: see Spatial Audio below |
| Automix, Smart Queue, Listening Mode | Sona has no such features: no tiles, rather than tiles that pretend |
| — | Sona's own: private listening (`private`) |

## Spatial Audio

CiderDeck's Atmos tile switches Cider 4's playback of Apple's immersive assets. Sona can do the same for most songs, as a feature of its own (its own branch and issue), after which the plugin gains the tile.

`playspike -probe-lossless` on 21 Atmos songs (gb storefront, CDM 4.10.3112.0, 2026-11-17): 13 (62%) get a Widevine key `c24` that the binaural AAC, the downmix AAC and both Atmos E-AC-3 variants share; the other 8 are FairPlay-only in every variant. ALAC (`c23`) is refused (`-1002`) on all 13. Whether a song is licensable is per song, not per label.

- **First: Apple's binaural AAC** (`*-binaural`): Apple's own Spatial Audio render for headphones, stereo AAC, through the existing decode lanes, gapless and crossfade intact. Needs: picking the variant from `extendedAssetUrls.enhancedHls`, `cbcs` decryption of audio samples (the shim's `Decrypt` takes `cbcs`; unproven on audio), and the `ctrp256` path for songs without the key.
- **Later, if wanted: Atmos E-AC-3** (`audio-atmos-*`). `avdec_eac3` decodes the 5.1 bed, not the objects; it needs multichannel output, a virtualiser on headphones, and a check of the decoder in the bundled GStreamer on Windows and macOS.

## Order

1. `feat(cli)`: the commands and status fields above, their add-on permissions (`queue.control`, `library`), docs. Done: #544.
2. `feat(cli)`: `sona connect`, the stdio bridge with v2 framing and the `status`, `now-playing` and `queue` topics (the queue polled every 2 s while watched, as `addons dev` polls its log). Done: #553.
3. The plugin repository: [coredev-uk/sona-streamdeck](https://github.com/coredev-uk/sona-streamdeck). Every action is driven against a running Sona by its `scripts/smoke.ts`, which plays the Stream Deck app's part; it has not yet run on a physical Stream Deck.
4. Separately: Spatial Audio (binaural AAC), then its tile. Done: #549, #557, and the plugin's Spatial Audio key.
