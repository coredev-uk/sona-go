# Sona's developer commands, for Linux, macOS and Windows: `just <recipe>`
# (`just` alone lists them). devenv's scripts are `just <recipe>` too
# (devenv.nix), so this file is the one definition of each command;
# docs/development.md ("Developer commands") describes them.
#
# Every recipe runs in a POSIX shell, so its text is the same on every
# system: sh on Linux and macOS, Git for Windows' sh on Windows (also when
# you run `just` from PowerShell). Your arguments reach the commands as you
# typed them ("$@"). What differs per system is explicit below: `exe`,
# `dbus` and the recipes that need Linux.
#
# The Go environment comes from outside: devenv on Linux and macOS, the cgo
# environment of docs/platforms.md on Windows. Packaging stays in
# Taskfile.yml.

set positional-arguments

# Git for Windows' default location. Elsewhere: `just --shell <sh.exe>
# --shell-arg -cu` (docs/development.md, "On Windows").
[windows]
set shell := ["C:/Program Files/Git/bin/sh.exe", "-cu"]

[windows]
set script-interpreter := ["C:/Program Files/Git/bin/sh.exe", "-eu"]

# Executables built here: bin\devloop.exe and bin\sona.exe on Windows.
exe := if os() == "windows" { ".exe" } else { "" }

# On Linux the tests run in a private D-Bus session: no test reaches yours.
dbus := if os() == "linux" { "dbus-run-session -- " } else { "" }

# Lists the recipes.
[private]
default:
    @just --list

# The development loop (cmd/devloop): rebuilds the app with the dev tag on
# every save, restarts it where it was, reloads tokens and catalogues without a
# rebuild. `r` rebuilds, `q` quits. A first argument
# `-background` is devloop's: restarts don't take the focus.
[doc("Runs the app, rebuilding it on every change: r rebuilds, q quits")]
[script]
dev *args:
    go build -o bin/devloop{{ exe }} ./cmd/devloop
    flags=
    if [ "${1:-}" = -background ]; then flags=-background; shift; fi
    exec bin/devloop{{ exe }} $flags -- "$@"

# Every kit component in light and dark, under the same loop.
[script]
gallery *args:
    go build -o bin/devloop{{ exe }} ./cmd/devloop
    flags=
    if [ "${1:-}" = -background ]; then flags=-background; shift; fi
    exec bin/devloop{{ exe }} -target ./cmd/uigallery $flags -- "$@"

# No inspector (mygo_noinspector) and MyGo's production mode (build/ldflags.sh;
# without the Last.fm and Discord credentials: no -trimpath here, which would
# leave them in the build info).
[doc("Builds the production binary, bin/sona (bin\\sona.exe on Windows)")]
build *args:
    go build -tags mygo_noinspector,sqlite_fts5 -ldflags "$(bash build/ldflags.sh --no-credentials --goos "$(go env GOOS)")" -o bin/sona{{ exe }} ./cmd/sona "$@"

# The documentation site (site/, Astro Starlight) with live reload.
docs *args:
    cd site && npm ci && npm run dev -- "$@"

# sqlc into internal/db/dbq. Commit the result.
gen:
    sqlc generate -f internal/db/sqlc.yaml

# xargs keeps each command line under the system's limit (Windows' is 32 KiB).
[doc("gofumpt on every tracked Go file except internal/db/dbq")]
fmt:
    git ls-files '*.go' | grep -v '^internal/db/dbq/' | xargs gofumpt -w

[doc("The static checks: vet, type-checks, lint, mygo vet, catalogues, path case, gofumpt")]
[script]
check:
    go vet ./...
    # Development builds (devloop's restore, the instance names, probe
    # builds) type-check too.
    go vet -tags dev,sqlite_fts5 ./...
    go vet -tags probe,sqlite_fts5 ./...
    # Every package must still type-check on the other platforms
    # (docs/platforms.md, "Type-check and cross-build").
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
    go run ./cmd/typecheck -os darwin -arch arm64 ./...
    golangci-lint run --new-from-rev=origin/main ./...
    # Build values kept past their frame (MyGo's lifetime check).
    go tool mygo vet internal/ui
    # Both message catalogues match the source (docs/translations.md,
    # "Add or change a UI string").
    go run ./cmd/i18ntool extract -check
    go run ./cmd/pathcase
    # Every tracked Go file is formatted the way `fmt` leaves it (#409).
    unformatted=$(git ls-files '*.go' | grep -v '^internal/db/dbq/' | xargs gofumpt -l)
    if [ -n "$unformatted" ]; then
      echo "check: not gofumpt-formatted (run fmt):" >&2
      echo "$unformatted" >&2
      exit 1
    fi

# Not `test`: in devenv the shell's `test` builtin would shadow it.
[doc("go test -race, then the tests of dev-only code in a development build")]
[script]
tests *args:
    # go test's 10 minute default is a wall-clock limit a busy machine can pass
    # in the UI packages (docs/testing.md, "Timing in tests"): 30 minutes only
    # guards against a hang. An argument of yours comes later and wins.
    {{ dbus }}go test -race -timeout 30m ./... "$@"
    {{ dbus }}go test -race -timeout 30m -tags dev,sqlite_fts5 ./internal/devinstance ./internal/ui/devstate ./internal/ui/app ./internal/ui/features/song ./internal/ui/features/discover ./internal/app ./internal/mpris

# The Apple sign-in flow from the terminal.
auth-spike *args:
    go run ./cmd/authspike "$@"

# The DRM path regression (needs MUSIC_USER_TOKEN; -preview needs none).
play-spike *args:
    go run ./cmd/playspike "$@"

# In `headless` on a profile whose covers are cached or reachable (the e2e
# fixture's are not, outside e2e).
[doc("The Albums scroll probe in the app's own process (docs/performance.md)")]
[script]
probe *args:
    go build -tags probe,sqlite_fts5 -o bin/sona-probe{{ exe }} ./cmd/sona
    SONA_PRIVATE=1 exec bin/sona-probe{{ exe }} -probe "$@"

# Private XDG dirs, D-Bus, AT-SPI and stub portals (docs/testing.md, "UI tests
# and the end-to-end smoke").
[doc("Runs a command in a private headless sway session: headless -- <cmd> (Linux)")]
headless *args: (_linux "headless")
    bash e2e/headless.sh "$@"

# The end-to-end smoke and the accessibility check, in `headless` (Linux).
e2e *args: (_linux "e2e")
    bash e2e/headless.sh -- go test -tags e2e -count=1 ./e2e/... "$@"

# site/src/content/docs/contributing/accessibility.md, "Listening with Orca".
# Orca comes from the pinned nixpkgs and is built on first use, so it is not
# part of everyone's dev shell; what it said is kept in bin/orca-out (E2E_OUT).
[doc("Walks the app with a real Orca, in headless (Linux)")]
[script]
orca-walk *args: (_linux "orca-walk")
    rev=$(sed -n 's|.*nixpkgs/\([0-9a-f]\{40\}\)$|\1|p' devenv.yaml)
    orca=$(nix build --no-link --print-out-paths "github:NixOS/nixpkgs/$rev#orca")/bin/orca
    E2E_ORCA=1 E2E_ORCA_BIN=$orca E2E_OUT=${E2E_OUT:-$PWD/bin/orca-out} bash e2e/headless.sh -- go test -tags e2e -count=1 -v -run '^TestOrca' ./e2e/ "$@"

# Built and smoke-tested in podman/docker on Ubuntu 24.04.
[doc("The Linux release packages, .deb/.rpm/Arch/AppImage (Linux): build/linux/release/release.sh")]
release *args: (_linux "release")
    bash build/linux/release/release.sh "$@"

# Stops a Linux-only recipe elsewhere, with the reason.
[no-exit-message]
[private]
_linux recipe:
    @[ "{{ os() }}" = linux ] || { echo "just {{ recipe }}: needs Linux; run it there, in devenv." >&2; exit 1; }
