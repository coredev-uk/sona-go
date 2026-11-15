# Performance and memory

How to measure the UI's frame cost, start-up and memory. Run every measurement headless and private (see [development.md](development.md)).

## Contents

- [Measure UI performance](#measure-ui-performance)
  - [Read frame times](#read-frame-times)
  - [Build the probe](#build-the-probe)
  - [What the scroll does](#what-the-scroll-does)
  - [Run the probe](#run-the-probe)
  - [Compare runs](#compare-runs)
- [Measure artwork fetch times](#measure-artwork-fetch-times)
- [Measure memory](#measure-memory)
  - [Read the diagnostics bundle](#read-the-diagnostics-bundle)
  - [Script a run with memprobe](#script-a-run-with-memprobe)
  - [Make two runs comparable](#make-two-runs-comparable)

## Measure UI performance

MyGo draws every frame itself, so frame cost is measured in the process, not in a browser.

### Read frame times

| Setting | Effect |
| --- | --- |
| `MYGO_FRAME_STATS=all` | MyGo logs one line per presented frame: `mygo: frame N took … ms: build …, layout …, paint …, present …`. |
| `MYGO_FRAME_STATS=8` | Logs only the frames slower than 8 ms. A scroll through Albums or Home should not log any on a desktop GPU. |
| `SONA_INSPECTOR=1` | Opens MyGo's inspector beside the content (element boxes, styles, frame times). |

`startup: first paint` in `<state>/sona.log` is the time from process start to the first presented frame. Compare several starts, not one.

### Build the probe

```sh
go build -tags probe,sqlite_fts5 -o bin/sona-probe ./cmd/sona
```

The probe build links MyGo's automation plugin (`cmd/sona/automation_probe.go`). It is never a release build, and never built with `mygo_noinspector`, which drops the plugin.

The plugin listens only when `MYGO_AUTOMATION=<socket path>` is set. It drives the app through its accessibility tree and input events, as a touchpad or a screen reader would.

Two tools use it and print the same table:

- **`-probe`** measures the Albums scroll in the app's own process and quits:

  ```sh
  bin/sona-probe -probe [-speed 2000] [-max 20000] [-runs 3] [-idle 10s] [-interval ms]
  ```

  `-startup` quits once the covers are on screen. The probe starts the app again with `MYGO_FRAME_STATS=all` and a private automation socket (MyGo reads both while its packages initialise), takes the frame lines off the log, and scrolls through wheel events over that socket.
- **`uiprobe`** does the same from outside, against a probe build you started with `MYGO_AUTOMATION=… MYGO_FRAME_STATS=all`:

  ```sh
  go run ./cmd/uiprobe -socket $MYGO_AUTOMATION -log $XDG_STATE_HOME/sona/sona.log
  ```

  The frame lines are then in `sona.log`; the pid and the process start come from the log's `startup: process start` line.
  `uiprobe -firstpaint run1/sona.log run2/sona.log …` summarises `startup: first paint` over several starts (min, median, max).
  Home, New and the motion artwork scenarios of v0.2.0's uiprobe come back once those pages exist.

### What the scroll does

The measurement lives in `internal/ui/probe/measure`. It runs these steps:

1. Wait until the tiles in view show pictures. The probe samples the window's capture at the top left of each tile (a cell taller than wide inside the page's scroll container): a decoded cover has far more colours than the placeholder. This is the start-up line's "covers on screen".
2. Wait until no frame has been drawn for 500 ms and the process tree is idle.
3. Take the display's frame interval (below).
4. Scroll down and back up at `-speed` DIPs per second over `-max` DIPs (the page's end if nearer), `-runs` times.
5. Watch the process at rest for `-idle`.

The frame interval comes from the first source that answers:

- `-interval`.
- The refresh rate of the output the window is on, asked of sway over `SWAYSOCK`. wlroots' headless output reports a refresh of 0 until the sway config sets a mode with one, `output HEADLESS-1 mode --custom 2560x1440@60Hz`, so without it the probe measures.
- A measurement from idle frames: the pointer moving on and off a sidebar link draws one small frame per refresh. Never from frames that redraw the grid, which would stretch the interval and hide dropped frames.

A value more than 10% from 16.67 ms is warned about.

A frame exists only when something changed, so a gap of more than 1.5 intervals between two frames counts the intervals it spans as dropped frames, as v0.2.0's probe did for `requestAnimationFrame`.

The table has:

- the gap p50 and p95, dropped frames and frames over 50 ms;
- the CPU of the process and its children while scrolling and at rest;
- MyGo's CPU time per frame (build, layout and paint) and present time;
- RSS and PSS at rest.

The first row is a control: the same wheel stream over the top bar, where nothing scrolls, which is what the input path costs. Per-pass lines go to stderr; the median of the passes is the table.

The probe always ends:

- If the covers are not drawn within 60 s (offline, an empty artwork cache) it prints what it has and fails with status 1 without scrolling.
- An unanswered automation request times out after a minute, and the whole probe has an overall limit.
- An app that does not shut down within 15 s of the probe is stopped with status 1.

`uiprobe` fails the same way.

### Run the probe

Run it headless, as every automated run: private XDG dirs, `dbus-run-session` and `SONA_PRIVATE=1`. The `headless` script wraps the compositor.

Use a sway headless output with the window floated at the size the baseline used:

```sh
swaymsg '[app_id="dev.coredev.Sona"] floating enable, resize set 1132 1253, move position 0 0'
```

A `for_window` rule in the sway config does the same. A library of the size you compare against matters: the scroll covers `min(-max, the page)` DIPs, and the frames per pass follow from it.

### Compare runs

- Compare before and after on the same machine, with the same library (about 650 albums makes a useful grid), the same window size and the same `-speed`.
- Confirm on a desktop session. The headless session draws on the CPU (llvmpipe, `frames: drawn in memory` in the output), so its CPU figures are not a desktop GPU's and its frame times only show relative change.
- `MYGO_FRAME_STATS=all` itself costs a log line and a few counters per frame.
- `-probe` counts its own wheel driver in the CPU figures; `uiprobe` does not (its driver is another process).

## Measure artwork fetch times

A cover that is slow to appear is slow in one of three places: the CDN, the queue in front of the downloads (`Cache.fetchSem`, 16 at once) or composing (a playlist's 2×2 mosaic). Measure each on its own with a throwaway program that is not committed:

- **The CDN.** Take the artwork URLs from the library's first pages (`applemusic.Client.LibraryPlaylistsPage`, `LibraryAlbumsPage` with the user token from `.env`, read-only), fill `{w}x{h}` and time `GET` to the last byte, with `Accept: image/jpeg,image/png;q=0.9,*/*;q=0.8` and `artwork.DefaultUserAgent`. Request each size once per artwork: a repeat is a cache hit. The `X-Cache` header says `hit-fresh` or `miss` at the CDN's edge, but an origin render shows in the time. Use 8 or more artworks per size and report the median and how many took over 150 ms. Never print playlist names or URLs.
- **The queue.** Start a fresh `artwork.Cache` (empty directory), ask for 60 album covers at once and time one more cover (a generated playlist cover and a mosaic) asked after 50 ms, against the same cover with nothing else loading.
- **Composing.** Time `Cache.Get` of an `artwork.Mosaic` of four albums on an empty cache directory, then again for the disk hit; the difference to the slowest of its four tiles is the compose cost.

Results of 2026-11-15 (one library, 24 playlists, 16 of them generated covers): a generated cover took 13 ms at 64, 256 and 512 px and 24 ms at 1200, 250 to 430 ms at every other size up to 1000 (128 px 252 ms, 1024 px 430 ms) and 975 ms at 2048. A mosaic composed on an empty cache took 93 to 150 ms once its tiles were downloaded (64 ms at 64 px); the tiles are 4 parallel fetches, so a mosaic waits for the slowest: albums at 128 px were slow (over 150 ms, up to 1.7 s) for 25 of 98 first requests. With 60 album covers loading, a generated cover took 217 ms and a mosaic 278 ms against 22 ms and (a fresh connection) 1.99 s alone: the 16-slot queue costs about one round trip, not the problem.

After moving the sources to 64, 256, 512, 1200 and 2048 px (empty cache directory per playlist, first picture, median of 8 generated covers): a 128 px box 331 ms before and 51 ms after, a 600 px box 593 ms before and 126 ms after; 40 and 300 px boxes were already on cached sizes (63 and 71 ms before, 61 and 62 ms after). Mosaics (12 each, connection warm) stayed at 67 to 94 ms at 128 px and 76 to 78 ms at 256 px: the slow tail of a 128 px album tile (25 of 98) is gone by construction, but 12 samples did not show it in either run.

## Measure memory

Memory is measured per process, never as one figure. The app and the playback host child (the Widevine CDM, GStreamer) are two rows in Activity Monitor and in `top`, and "400 MB" means different fixes depending on which holds it.

### Read the diagnostics bundle

Settings → Diagnostics → Copy diagnostics has a **Memory** section for exactly this (`internal/services/system_memory.go`, `internal/procmem`). For the app and the running host it shows:

- the resident size and the footprint the platform's own tools show (macOS: `proc_pid_rusage`'s physical footprint, Activity Monitor's Memory column; Linux: the proportional set size);
- the Go runtime's account of the app: heap in use, idle and returned to the OS.

A report from a Mac that pastes this section tells which process holds the memory, and whether it is Go's heap (Sona can shrink it) or native memory (the GPU's buffers, GStreamer, the CDM).

### Script a run with memprobe

`cmd/memprobe` measures the same on Linux, with a scripted run against a probe build.

The probe build writes the Go heap profile and the runtime's figures into `SONA_MEMPROFILE_DIR` on `SIGUSR1` (`internal/memprobe`). The host child cannot write files, so it logs its figures into `sona.log`.

memprobe signals the whole process tree, reads each process's `/proc/<pid>/smaps` (RSS, PSS, anonymous memory, the mappings that hold the most) and prints one JSON line per process:

```sh
SONA_MEMPROFILE_DIR=$PWD/prof MYGO_AUTOMATION=$PWD/a.sock SONA_PREVIEW=1 SONA_PRIVATE=1 bin/sona-probe &
go run ./cmd/memprobe -pid $! -socket $PWD/a.sock -dir $PWD/prof -o $PWD/keep \
    link:Discover wait:20s press:"Play Perfecth" wait:15s press:"Open now playing" wait:15s measure:now-playing
go tool pprof -sample_index=inuse_space -top bin/sona-probe keep/now-playing/<pid>.heap.pb.gz
```

Steps:

- `wait:<duration>`
- `link:`, `button:`, `toggle:`, `tab:` and `press:<label>`, through the accessibility tree
- `wheel:<dy>` and `scroll:<n>x<dy>`
- `measure:<name>`
- `dump` (the tree's labels)
- `shot:<file>`

Run it as every automated run:

- `headless -- …` with private XDG dirs and `SONA_PRIVATE=1`.
- Audio to your own PipeWire null sink. `PIPEWIRE_REMOTE` and `PULSE_SERVER` as absolute paths to the real sockets reach it from the sandboxed child; a symlink in the private runtime directory does not.
- A copy of a real library for the Albums scrolls.

### Heap returned after artwork bursts

- Opening a page with many covers decodes and resizes hundreds of buffers in a burst. The Go heap's idle pages stay resident for minutes afterwards, because the runtime paces against the peak.
- `internal/artwork` runs `debug.FreeOSMemory` once, 5 s after the last in-flight artwork operation ends. It is one collection per burst, never per frame.
- Read it as `HeapReleased` close to `HeapIdle` in the memprobe figures a few seconds after a page has loaded.

### Make two runs comparable

Three things decide whether two runs compare:

- **The GL driver.** libglvnd picks NVIDIA's EGL on some runs and Mesa's on others (NVIDIA's mapped 60 to 70 MB more).
  - Pin Mesa's with `__EGL_VENDOR_LIBRARY_FILENAMES=…/50_mesa.json GALLIUM_DRIVER=llvmpipe MYGO_GPU=1`. The headless session draws on llvmpipe, whose textures are ordinary process memory, a stand-in for a unified-memory GPU.
  - Run each scenario three times: PSS moves by ±15 MB from run to run with what Mesa touched.
- **The scale.** The headless output is 1x. `swaymsg output HEADLESS-1 scale 2` before starting the app gives the 2x of a Retina Mac, where a cover is four times the bytes.
- **Linux is not macOS.** Mesa's libraries (50 MB), GTK and the GL texture cache are Linux-only costs.
  - On macOS MyGo draws with Metal, drops every image texture once the window has been idle for 2 s and keeps up to three window-sized frames while it animates.
  - What carries over to macOS is Go's heap, the artwork and bitmap caches, the GStreamer pipelines and the CDM host.
  - The rest needs a Mac, and the diagnostics bundle above is how to get its numbers.
