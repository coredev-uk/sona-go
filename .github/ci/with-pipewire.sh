#!/usr/bin/env bash
# with-pipewire.sh COMMAND...: runs COMMAND with a PipeWire daemon and
# WirePlumber of its own in a private XDG_RUNTIME_DIR (the playback tests look
# for $XDG_RUNTIME_DIR/pipewire-0 and make null sinks with pw-cli). Start it
# inside `dbus-run-session --` so the daemons share COMMAND's session bus.
set -euo pipefail
XDG_RUNTIME_DIR=$(mktemp -d)
export XDG_RUNTIME_DIR
chmod 700 "$XDG_RUNTIME_DIR"
pids=()
trap 'kill ${pids[@]+"${pids[@]}"} 2>/dev/null || true; rm -rf "$XDG_RUNTIME_DIR"' EXIT
pipewire &
pids+=($!)
for _ in $(seq 100); do
	[[ -S $XDG_RUNTIME_DIR/pipewire-0 ]] && break
	sleep 0.1
done
[[ -S $XDG_RUNTIME_DIR/pipewire-0 ]] || {
	echo "::error::pipewire did not start"
	exit 1
}
wireplumber &
pids+=($!)
"$@"
