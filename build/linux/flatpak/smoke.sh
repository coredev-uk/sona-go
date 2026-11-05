#!/usr/bin/env bash
# Smoke test for the installed Sona Flatpak (build.sh installs it --user).
#
#   devenv shell -- bash build/linux/flatpak/smoke.sh
#
# Runs the installed app in e2e/headless.sh's private desktop session (sway,
# its own session bus, a private HOME and XDG dirs, SONA_PRIVATE=1) and passes
# when
#   - `flatpak run dev.coredev.Sona --version` prints VERSION,
#   - the app logs "startup: first paint" and sway shows its window,
#   - the Widevine CDM downloads into the Flatpak's own data directory
#     (needs network; SONA_FLATPAK_SMOKE_OFFLINE=1 skips it),
#   - org.mpris.MediaPlayer2.sona and the single-instance name are owned on the
#     session bus, and a second `flatpak run` reaches the running instance.
# Playback is not covered: it needs a PipeWire sink (docs/flatpak.md, "Smoke
# test").
set -euo pipefail

root=$(cd "$(dirname "$0")/../../.." && pwd)
self=$(readlink -f "${BASH_SOURCE[0]}")

if [[ -z ${HEADLESS_ROOT:-} ]]; then
  # The user installation lives under the real XDG data dir; headless.sh moves
  # it, so pin it first.
  export FLATPAK_USER_DIR=${FLATPAK_USER_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/flatpak}
  exec bash "$root/e2e/headless.sh" -- bash "$self"
fi

fail() {
  echo "flatpak smoke: FAIL: $*" >&2
  exit 1
}

# devenv's LD_LIBRARY_PATH is the app's GTK stack, which flatpak must not load.
fp() {
  if command -v flatpak >/dev/null 2>&1; then
    env -u LD_LIBRARY_PATH flatpak "$@"
  else
    env -u LD_LIBRARY_PATH nix shell nixpkgs#flatpak nixpkgs#bubblewrap -c flatpak "$@"
  fi
}

# ~/.var/app/dev.coredev.Sona is under $HOME: keep it private.
export HOME=$HEADLESS_ROOT/home
mkdir -p "$HOME"
want=$(<"$root/VERSION")

got=$(fp run dev.coredev.Sona --version 2>/dev/null) || fail "flatpak run --version failed"
[[ $got == "sona $want" ]] || fail "--version printed '$got', want 'sona $want'"
echo "flatpak smoke: $got"

# MYGO_GPU=0: headless sway has no GPU buffers (CPU renderer, as the release
# smoke test).
fp run --env=MYGO_GPU=0 dev.coredev.Sona >"$HEADLESS_ROOT/app.out" 2>&1 &
app=$!
trap 'fp kill dev.coredev.Sona >/dev/null 2>&1 || true; kill "$app" 2>/dev/null || true' EXIT

log=$HOME/.var/app/dev.coredev.Sona/.local/state/sona/sona.log
for _ in $(seq 1 60); do
  grep -q '"startup: first paint"' "$log" 2>/dev/null && break
  sleep 0.5
done
grep -q '"startup: first paint"' "$log" 2>/dev/null || { cat "$HEADLESS_ROOT/app.out"; fail "no first paint"; }
grep -o '"msg":"startup: first paint"[^}]*' "$log"
swaymsg -t get_tree | grep -q '"sandbox_app_id": "dev.coredev.Sona"' || fail "sway has no window from the Flatpak"

names=$(dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.ListNames)
grep -q 'org.mpris.MediaPlayer2.sona' <<<"$names" || fail "org.mpris.MediaPlayer2.sona is not owned"
grep -q '"dev.coredev.Sona"' <<<"$names" || fail "dev.coredev.Sona (single instance) is not owned"
fp run --command=sona dev.coredev.Sona status >/dev/null 2>&1 || fail "a second flatpak run does not reach the running instance"

if [[ -z ${SONA_FLATPAK_SMOKE_OFFLINE:-} ]]; then
  data=$HOME/.var/app/dev.coredev.Sona/data/sona/widevine
  for _ in $(seq 1 120); do
    [[ -f $data/current.json ]] && break
    sleep 1
  done
  [[ -f $data/current.json ]] || fail "the Widevine CDM did not download into $data"
  echo "flatpak smoke: Widevine installed under the Flatpak's data dir: $(basename "$(printf '%s\n' "$data"/*/ | head -1)")"
fi
echo "flatpak smoke: PASS"
