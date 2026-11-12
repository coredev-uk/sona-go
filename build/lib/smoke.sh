# shellcheck shell=bash disable=SC2154
# Shared by the smoke scripts (build/darwin, build/windows, build/linux/release,
# build/linux/flatpak): source it, set smoke_name first ("smoke(darwin)").
#
#   fail MSG...             prints "<smoke_name>: FAIL: MSG" on stderr and exits 1
#   version                 prints what the artefact must report: $VERSION, else the VERSION file
#   check_version WANT CMD...  `CMD --version` must print "sona WANT"
#   wait_for_log LOG PATTERN SECS [PID]
#                           polls once a second, up to SECS, until PATTERN (an
#                           extended regexp) is in LOG; fails at once when PID
#                           (the app) has exited. A final look follows the last
#                           wait, so the status says whether PATTERN is there.
#   stop_app PID            ends the app and waits for it

fail() {
  echo "$smoke_name: FAIL: $*" >&2
  exit 1
}

smoke_repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
version() {
  echo "${VERSION:-$(cat "$smoke_repo/VERSION")}"
}

check_version() {
  local want=$1 got
  shift
  got=$("$@" --version) || fail "$* --version failed"
  [[ $got == "sona $want" ]] || fail "$* --version printed '$got', want 'sona $want'"
  echo "$smoke_name: $got"
}

wait_for_log() {
  local log=$1 pattern=$2 secs=$3 pid=${4:-} i
  for ((i = 0; i < secs; i++)); do
    grep -qE "$pattern" "$log" 2>/dev/null && return 0
    [[ -z $pid ]] || kill -0 "$pid" 2>/dev/null || break
    sleep 1
  done
  grep -qE "$pattern" "$log" 2>/dev/null
}

stop_app() {
  kill "$1" 2>/dev/null || true
  wait "$1" 2>/dev/null || true
}
