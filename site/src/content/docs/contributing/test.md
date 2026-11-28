---
title: Test Your Change
description: Run the checks and tests, update golden images, and run the end-to-end and live-playback checks.
---

Run these in the [devenv environment](/contributing/setup/), or as `just <command>` on Windows (`just check`, `just tests`). Run `check` and `tests` before every pull request.

## Run the checks

```sh
devenv shell -- check
```

`check` runs the static half: `go vet` (also for the `dev` and `probe` builds), the Windows and macOS type-checks, `golangci-lint` on the whole tree, `mygo vet internal/ui`, the translation catalogue check, the path-case check and the gofumpt checks. Fix what it reports. CI runs the same commands on every pull request, so what fails `check` fails there too.

## Run the tests

```sh
devenv shell -- tests
```

`tests` runs `go test -race ./...` in a private D-Bus session, including the `ui.Tester` tests and the golden images. Extra arguments go to `go test`.

To run one test:

```sh
devenv shell -- go test -race ./internal/<pkg> -run '^TestName$'
```

## Test UI behaviour

`ui.Tester` renders views without a window: it drives keys and clicks and takes image snapshots. `uitest.Env(t)` gives a test a `module.Env` over real services on a temporary library of invented data. Never use Apple's data in a fixture.

### Update golden images

Golden images are a few shell screenshots, light and dark, in `internal/ui/app/testdata/golden/`. They tolerate another machine's text rendering but not a moved edge or a changed colour. When a change moves the picture on purpose:

1. Review the new picture.
2. Run `devenv shell -- go test ./internal/ui/app -run Golden -update`.
3. Commit the new PNGs and say why in the pull request.

A failing run keeps the picture and a diff in the test's artifact directory. If your machine's text differs from CI's, ask a maintainer to regenerate the goldens on your pull request, or see [docs/testing.md](https://github.com/coredev-uk/sona-go/blob/main/docs/testing.md) to update them in the CI image.

## Run the end-to-end tests

```sh
devenv shell -- e2e
```

`e2e` drives the built app from outside, on a fixture library, and checks first paint, the Albums page, playing an album and the accessibility tree. Nothing reaches Apple.

To run any command in a private headless session (sway, private XDG directories, its own session bus), use `headless`:

```sh
devenv shell -- headless -- <command>
```

## Check live playback

Run live checks in private mode, with audio going to your own PipeWire null sink, never the default sink or the microphone.

1. Create a sink:

   ```sh
   pw-cli create-node adapter '{ factory.name=support.null-audio-sink node.name=sona-test media.class=Audio/Sink object.linger=true audio.position=[FL FR] }'
   ```

2. Play to it (needs `MUSIC_USER_TOKEN` in `.env`):

   ```sh
   SONA_PRIVATE=1 devenv shell -- go run ./cmd/playspike -device sona-test -seek 90
   ```

3. Destroy the sink you created: `pw-cli destroy sona-test`.

Run `playspike -seek 90` after any change to the CDM, playback or Apple requests. Without a token, use `SONA_PREVIEW=1` or `playspike -preview` (see [Build and Run](/contributing/build-and-run/)).

If WirePlumber stops linking streams, ask before restarting it.

## Go further

[docs/testing.md](https://github.com/coredev-uk/sona-go/blob/main/docs/testing.md) covers preview playback, the real-CDM tests, desktop VM tests and secret scanning. [docs/performance.md](https://github.com/coredev-uk/sona-go/blob/main/docs/performance.md) covers frame and memory measurements. For accessibility checks see [UI Conventions](/contributing/ui-conventions/).
