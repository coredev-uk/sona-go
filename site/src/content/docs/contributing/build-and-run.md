---
title: Build and Run
description: Run Sona from a checkout, with live reload, the component gallery, previews and private mode.
---

Run these in the [devenv environment](/contributing/setup/): prefix each command with `devenv shell --`, or use direnv.

## Run with live reload

```sh
devenv shell -- dev
```

`dev` builds a development build into `bin/sona-dev`, runs it, and rebuilds and restarts it when a Go file changes. The restart returns to the same page, scroll position and window.

- Press `r` to rebuild and `q` to quit.
- Run `dev -background -- <app args>` to start the app without taking focus.
- Design tokens (`internal/ui/theme/tokens.json`) and `.po` catalogues reload in the running app with no build.
- Set `SONA_INSPECTOR=1` to open MyGo's inspector (F12 or Ctrl+Shift+I toggles it).

A development build starts in [private mode](#use-private-mode). A plain `go build ./cmd/sona` is not a development build; only `-tags dev` is.

## Browse the component gallery

```sh
devenv shell -- gallery
```

The gallery shows every kit component in light and dark, with knobs, and restarts like `dev`. A kit change adds its own section in `cmd/uigallery`.

## Build a release-style binary

```sh
devenv shell -- build
```

This writes `bin/sona`: no inspector, MyGo in production mode. A build that sets its own `-tags` must name `sqlite_fts5` too, because `-tags` replaces devenv's `GOFLAGS`.

## Play without an account

`SONA_PREVIEW=1` plays every song as its 30-second preview, with no account, no Widevine CDM and no licence. The player bar shows "Preview" beside the title.

```sh
SONA_PRIVATE=1 SONA_PREVIEW=1 devenv shell -- bin/sona
```

For the command line, `go run ./cmd/playspike -preview` plays a preview and watches it advance.

## Use private mode

Private mode stops Sona reporting plays to Apple and scrobbling them. Anything that plays music runs in it.

| Setting | Effect |
| --- | --- |
| Development build | Starts private, whatever the saved setting says. |
| `SONA_PRIVATE=1` | Forces private mode in any build. |
| `SONA_PRIVATE=0` | A development build follows the saved setting, for a deliberate live check. |

## Keep your session and library safe

Only one Sona may own your real session bus. For anything that doesn't need your desktop, start the app under `dbus-run-session` with private XDG directories, so it neither clashes with a running Sona nor touches your real settings and library:

```sh
env XDG_CONFIG_HOME=$PWD/.tmp/config XDG_DATA_HOME=$PWD/.tmp/data \
    XDG_STATE_HOME=$PWD/.tmp/state XDG_CACHE_HOME=$PWD/.tmp/cache \
    dbus-run-session -- bin/sona
```

Set the XDG variables outside `dbus-run-session`, so the services the private bus starts use them too.

To run two development builds on one session bus, give each a name with `SONA_DEV_INSTANCE=<name>`.

## Preview the docs

`devenv shell -- docs` starts the documentation site. See [Writing Docs](/contributing/documentation/).

## Go further

[docs/development.md](https://github.com/coredev-uk/sona-go/blob/main/docs/development.md) lists every devenv script (`probe`, `release`, spikes) and describes the Widevine CDM. [docs/platforms.md](https://github.com/coredev-uk/sona-go/blob/main/docs/platforms.md) covers macOS and Windows.
