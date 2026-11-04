#!/usr/bin/env bash
# headless.sh [--] COMMAND [ARGS...]
#
# Runs COMMAND in a private, disposable desktop session, with no display, no
# real session bus and none of the user's files involved:
#
#   - a headless sway (wlroots headless backend, pixman renderer) on its own
#     Wayland socket; COMMAND gets WAYLAND_DISPLAY (and SWAYSOCK, for
#     swaymsg and grim);
#   - private XDG_CONFIG_HOME, XDG_DATA_HOME, XDG_STATE_HOME, XDG_CACHE_HOME
#     and XDG_RUNTIME_DIR under one temporary directory (removed afterwards;
#     the Go build and module caches are kept where they were);
#   - a session bus of its own (dbus-run-session), with the AT-SPI bus on it
#     (at-spi-bus-launcher), so GTK's accessibility bridge starts and a
#     client can read the app's accessibility tree;
#   - the desktop portals stubbed: MyGo asks the Settings portal for the
#     colour scheme and waits ~12 s for a service that never starts in a bare
#     session. A stub answers at once;
#   - a `wtype` keep-alive, which gives the sway seat a keyboard: without
#     one the app's window never gets keyboard focus.
#
# Usable as `headless -- go test -tags e2e ./e2e/...` (the devenv
# script of the same name runs this file; CI runs it directly). The exit
# status is COMMAND's. Needs sway, wtype, dbus-run-session, dbus-daemon,
# dbus-send, at-spi-bus-launcher (at-spi2-core) and go (the portal stub,
# e2e/portalstub, is built from the repository on each run).
set -euo pipefail

self=$(readlink -f "${BASH_SOURCE[0]}")
root_dir=$(cd "$(dirname "$self")/.." && pwd)

if [[ ${1:-} == --inner ]]; then
	shift
	inner=1
else
	inner=
fi
[[ ${1:-} == -- ]] && shift
if [[ $# -eq 0 ]]; then
	echo "usage: headless [--] COMMAND [ARGS...]" >&2
	exit 2
fi

die() {
	echo "headless: $*" >&2
	exit 127
}

need() { command -v "$1" >/dev/null 2>&1 || die "$1 not found ($2)"; }

# a11y_launcher prints the path of at-spi-bus-launcher: on PATH, where
# Debian, Ubuntu, Fedora and Arch put it, or next to the at-spi2-core library
# on LD_LIBRARY_PATH (Nix keeps it in libexec beside lib).
a11y_launcher() {
	local d p
	if command -v at-spi-bus-launcher >/dev/null 2>&1; then
		command -v at-spi-bus-launcher
		return 0
	fi
	for d in /usr/libexec /usr/lib/at-spi2-core /usr/lib/at-spi2 /usr/lib64/at-spi2-core /usr/lib/x86_64-linux-gnu/at-spi2-core /usr/lib/aarch64-linux-gnu/at-spi2-core; do
		[[ -x $d/at-spi-bus-launcher ]] && { echo "$d/at-spi-bus-launcher"; return 0; }
	done
	IFS=: read -r -a dirs <<<"${LD_LIBRARY_PATH:-}"
	for p in "${dirs[@]}"; do
		[[ -n $p && -x $p/../libexec/at-spi-bus-launcher ]] && { echo "$p/../libexec/at-spi-bus-launcher"; return 0; }
	done
	return 1
}

# ---- the session bus's side: AT-SPI, portals, the keyboard keeper, COMMAND.
if [[ -n $inner ]]; then
	pids=()
	cleanup_inner() { for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done; wait 2>/dev/null || true; }
	trap cleanup_inner EXIT

	launcher=$(a11y_launcher) || die "at-spi-bus-launcher not found (at-spi2-core)"
	"$launcher" --launch-immediately &
	pids+=($!)

	# Wait for the a11y bus: org.a11y.Bus answers on the session bus.
	for _ in $(seq 1 100); do
		if dbus-send --session --print-reply --dest=org.a11y.Bus /org/a11y/bus org.a11y.Bus.GetAddress >/dev/null 2>&1; then
			break
		fi
		sleep 0.1
	done

	# The registry of accessibles: the a11y bus's own activation does not find
	# it everywhere (Nix), and without it GTK's bridge cannot register the app.
	registryd=$(dirname "$launcher")/at-spi2-registryd
	if [[ -x $registryd ]]; then
		"$registryd" &
		pids+=($!)
		sleep 0.3
	fi

	# The portal stub owns org.freedesktop.portal.Desktop and answers the
	# Settings portal; built once into the private cache-less temp dir.
	(cd "$root_dir" && go build -o "$HEADLESS_ROOT/portalstub" ./e2e/portalstub) || die "building the portal stub failed"
	"$HEADLESS_ROOT/portalstub" &
	pids+=($!)
	for _ in $(seq 1 100); do
		if dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.NameHasOwner string:org.freedesktop.portal.Desktop 2>/dev/null | grep -q 'boolean true'; then
			break
		fi
		sleep 0.1
	done

	env -u LD_LIBRARY_PATH wtype -s 86400000 &
	pids+=($!)
	sleep 0.3

	set +e
	"$@"
	status=$?
	set -e
	exit "$status"
fi

# ---- the outside: sway and the private directories, then the session bus.
need sway "sway"
need wtype "wtype"
need dbus-run-session "dbus"
need dbus-daemon "dbus"
need dbus-send "dbus"
need go "Go: the portal stub is built from the repository"

# Short paths: Unix socket paths are limited to 108 bytes.
HEADLESS_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/sona-hl.XXXXXX")
export HEADLESS_ROOT

# The Go caches stay where they were: a private XDG_CACHE_HOME would make
# every run build from scratch.
GOCACHE=${GOCACHE:-$(go env GOCACHE)}
GOMODCACHE=${GOMODCACHE:-$(go env GOMODCACHE)}
export GOCACHE GOMODCACHE

export XDG_RUNTIME_DIR=$HEADLESS_ROOT/run
export XDG_CONFIG_HOME=$HEADLESS_ROOT/config
export XDG_DATA_HOME=$HEADLESS_ROOT/data
export XDG_STATE_HOME=$HEADLESS_ROOT/state
export XDG_CACHE_HOME=$HEADLESS_ROOT/cache
mkdir -p "$XDG_RUNTIME_DIR" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_STATE_HOME" "$XDG_CACHE_HOME"
chmod 700 "$XDG_RUNTIME_DIR"
# Nothing of the host's desktop may leak in.
unset DBUS_SESSION_BUS_ADDRESS DISPLAY SWAYSOCK WAYLAND_SOCKET I3SOCK
export XDG_SESSION_TYPE=wayland
export NO_AT_BRIDGE=0
export GTK_USE_PORTAL=0
export SONA_PRIVATE=1

cat >"$HEADLESS_ROOT/sway.conf" <<'EOF'
default_border none
default_floating_border none
titlebar_border_thickness 0
titlebar_padding 0
font pango:monospace 1
output HEADLESS-1 mode --custom 1920x1440@60Hz position 0 0
seat * hide_cursor 0
# The baseline's window: v0.2.0 was measured floating at 1132x1253.
for_window [app_id="dev.coredev.Sona"] floating enable, resize set 1132 1253, move position 0 0
EOF

sway_pid=
sway_real=
cleanup() {
	# $sway_pid is the wrapper's when sway is a wrapper script; the compositor
	# itself is the pid in its socket's name.
	[[ -n $sway_pid ]] && kill "$sway_pid" 2>/dev/null
	[[ -n $sway_real ]] && kill "$sway_real" 2>/dev/null
	wait 2>/dev/null || true
	rm -rf "$HEADLESS_ROOT"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# sway and wtype run without LD_LIBRARY_PATH: devenv's is the app's GTK stack,
# which a compositor from another nixpkgs must not load.
WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 WLR_RENDERER=pixman \
	env -u LD_LIBRARY_PATH sway -c "$HEADLESS_ROOT/sway.conf" >"$HEADLESS_ROOT/sway.log" 2>&1 &
sway_pid=$!

display=
for _ in $(seq 1 150); do
	kill -0 "$sway_pid" 2>/dev/null || { cat "$HEADLESS_ROOT/sway.log" >&2; die "sway exited"; }
	for s in "$XDG_RUNTIME_DIR"/wayland-*; do
		if [[ -S $s ]]; then
			display=$(basename "$s")
			break 2
		fi
	done
	sleep 0.1
done
[[ -n $display ]] || { cat "$HEADLESS_ROOT/sway.log" >&2; die "sway did not start"; }
export WAYLAND_DISPLAY=$display
for s in "$XDG_RUNTIME_DIR"/sway-ipc.*.sock; do
	if [[ -S $s ]]; then
		export SWAYSOCK=$s
		sway_real=$(basename "$s" .sock)
		sway_real=${sway_real##*.}
	fi
done

set +e
dbus-run-session -- "$self" --inner -- "$@"
status=$?
set -e
exit "$status"
