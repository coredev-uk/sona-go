#!/usr/bin/env bash
# Smoke test for the Windows artefacts (Git Bash on windows-2025, which has a
# desktop session).
#
#   build/windows/smoke.sh OUTDIR   # package.sh's output directory
#
# Passes when the portable zip's sona.exe prints the version and reaches
# "startup: first paint", and the installer installs silently with its files
# and the music:/itms: URL keys, and uninstalls them again. Uses a private
# profile; plays nothing (SONA_PRIVATE=1).
set -euo pipefail
export MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL="*" # Git Bash must not rewrite /S and /D=
out=$(cd "${1:?usage: smoke.sh OUTDIR}" && pwd)
version=${VERSION:-$(cat "$(dirname "$0")/../../VERSION")}
fail() { echo "smoke(windows): FAIL: $*" >&2; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
profile=$work/profile
mkdir -p "$profile/Roaming" "$profile/Local"
export SONA_PRIVATE=1 APPDATA="$profile/Roaming" LOCALAPPDATA="$profile/Local"

# 1. The portable zip.
# 7z is a native program: it wants Windows paths.
7z x -bso0 -bsp0 -o"$(cygpath -w "$work/zip")" "$(cygpath -w "$out/Sona-$version-windows-x64.zip")"
exe=$work/zip/Sona/sona.exe
got=$("$exe" --version) || fail "sona.exe --version failed"
[[ $got == "sona $version" ]] || fail "sona.exe --version printed '$got', want 'sona $version'"

log=$LOCALAPPDATA/Sona/State/sona.log
"$exe" >"$work/sona.out" 2>&1 &
pid=$!
for _ in $(seq 1 60); do
  grep -q '"startup: first paint"' "$log" 2>/dev/null && break
  kill -0 "$pid" 2>/dev/null || break
  sleep 1
done
painted=0
grep -q '"startup: first paint"' "$log" 2>/dev/null && painted=1
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
if [[ $painted != 1 ]]; then
  cat "$work/sona.out" "$log" 2>/dev/null || true
  fail "sona.exe never painted"
fi
echo "smoke(windows): portable zip reached first paint"

# 2. The installer, silently, into a folder of the test's own.
setup=$out/Sona-$version-windows-x64-setup.exe
dest=$work/installed
"$setup" /S "/D=$(cygpath -w "$dest")"
for f in sona.exe sona.ico uninstall.exe; do
  [[ -f $dest/$f ]] || fail "installer left no $f"
done
[[ $("$dest/sona.exe" --version) == "sona $version" ]] || fail "installed sona.exe prints the wrong version"
for scheme in music itms; do
  reg query "HKCU\\Software\\Classes\\$scheme\\shell\\open\\command" >/dev/null || fail "no $scheme: URL handler"
done
reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Sona' >/dev/null || fail "not listed under Apps"

# 3. The uninstaller removes the folder and the keys. It copies itself to a
# temporary folder to delete its own, so wait for the folder to go.
"$dest/uninstall.exe" /S
for _ in $(seq 1 30); do
  [[ -e $dest/sona.exe ]] || break
  sleep 1
done
[[ ! -e $dest/sona.exe ]] || fail "uninstall left sona.exe"
for scheme in music itms; do
  ! reg query "HKCU\\Software\\Classes\\$scheme" >/dev/null 2>&1 || fail "uninstall left the $scheme: keys"
done
echo "smoke(windows): PASS"
