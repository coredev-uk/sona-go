---
title: Set Up Your Environment
description: Install devenv, create a worktree and run commands in the project's toolchain.
---

Everything runs through [devenv](https://devenv.sh), which pins Go, GStreamer and the system libraries Sona links. You need [Nix](https://nixos.org/download) and devenv 2.4 or newer on Linux, or on a Mac with Apple silicon.

## Get the code

1. Fork [coredev-uk/sona-go](https://github.com/coredev-uk/sona-go) and clone your fork, or clone it directly if you can push branches.
2. Keep that checkout on `main`. Give every change its own worktree and branch, named `<type>/<slug>` after its [Conventional Commit](/contributing/pull-requests/) type:

   ```sh
   git worktree add ../sona-go-queue-autoplay -b feat/queue-autoplay origin/main
   cd ../sona-go-queue-autoplay
   ```

Git hooks refuse commits and pushes on `main`. Worktrees keep parallel changes apart, and each one needs its own private runtime state (see [Build and Run](/contributing/build-and-run/)).

## Enter the environment

Run one command in the toolchain:

```sh
devenv shell -- <command>
```

Or open an interactive shell with `devenv shell`. The first run downloads the toolchain. The shell sets `CGO_ENABLED=1`, `GOFLAGS=-tags=sqlite_fts5` and the GStreamer paths, and loads `.env`.

Without devenv installed, run it through Nix:

```sh
nix run github:NixOS/nixpkgs/nixpkgs-unstable#devenv -- shell
```

### Load it automatically with direnv

With [direnv](https://direnv.net), the checked-in `.envrc` loads the same environment when you `cd` into a checkout or worktree. Run `direnv allow` once in each. After that `go`, `dev`, `check` and the other scripts work without the `devenv shell --` prefix.

## Add your token

Most work needs no account. For live checks (`playspike`, the live tests), put your Music User Token in `.env`, which git ignores:

```sh
MUSIC_USER_TOKEN=...
```

A new worktree has no `.env`. Symlink the one from your main checkout only when you need a live token. On Windows, where devenv doesn't run, `devloop` passes `.env`'s variables to the app itself and, in a worktree without one, uses the main checkout's.

Never print, log or commit `.env` or a token. `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET` and `SONA_DISCORD_CLIENT_ID` can live there too; development builds read them at run time.

## Next steps

- [Build and Run](/contributing/build-and-run/) your change.
- [Test Your Change](/contributing/test/) before you open a pull request.
- Full reference, including every devenv script: [docs/development.md](https://github.com/coredev-uk/sona-go/blob/main/docs/development.md).
