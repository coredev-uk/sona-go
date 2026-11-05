#!/usr/bin/env bash
# session-bus.sh COMMAND...: runs COMMAND with a private D-Bus session bus at
# a fixed address, like `dbus-run-session --` but with the same
# DBUS_SESSION_BUS_ADDRESS every run. dbus-run-session picks a new random
# socket each time, `go test` records the environment variables a test reads,
# and a changed value means the test result is never taken from the cache.
set -euo pipefail
sock=/tmp/sona-ci-session-bus
rm -f "$sock"
pid=$(dbus-daemon --session --fork --print-pid --address="unix:path=$sock")
trap 'kill "$pid" 2>/dev/null || true; rm -f "$sock"' EXIT
export DBUS_SESSION_BUS_ADDRESS="unix:path=$sock"
"$@"
