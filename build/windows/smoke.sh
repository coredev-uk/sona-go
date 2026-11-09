#!/usr/bin/env bash
# Smoke test for the Windows artefacts (Git Bash on windows-2025, which has a
# desktop session).
#
#   build/windows/smoke.sh OUTDIR   # package.sh's output directory
#
# Passes when
#   - the portable zip holds sona.exe, the shim DLL, the GStreamer runtime and
#     the trimmed plugins, and no Widevine CDM, and its sona.exe prints the
#     version, reaches "startup: first paint" and readies the audio stack
#     (the CDM host child scans the bundled plugins);
#   - playspike.exe (OUTDIR/smoke-tools, from package.sh with
#     SONA_SMOKE_TOOLS=1), run beside that sona.exe, plays two 30-second
#     previews to fakesink: no Music User Token, no CDM;
#   - the update archive holds the same files;
#   - the installer installs silently with the same files and the music:/itms:
#     URL keys, and uninstalls them again.
# All of it runs with the installed GStreamer's bin\ taken off PATH and no
# GST_* variable set: the package must stand alone. Uses a private profile and
# SONA_PRIVATE=1.
set -euo pipefail
export MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL="*" # Git Bash must not rewrite /S and /D=
out=$(cd "${1:?usage: smoke.sh OUTDIR}" && pwd)
version=${VERSION:-$(cat "$(dirname "$0")/../../VERSION")}
fail() { echo "smoke(windows): FAIL: $*" >&2; exit 1; }

# The package stands alone: no GStreamer on PATH (a CI runner has the one
# the build linked), none of its variables, and the shim only where the
# package has it.
newpath=
IFS=: read -ra entries <<<"$PATH"
for p in "${entries[@]}"; do
  [[ ${p,,} == *gstreamer* ]] || newpath+=${newpath:+:}$p
done
export PATH=$newpath
for v in $(compgen -v GST_) SONA_CDM_SHIM MUSIC_USER_TOKEN; do unset "$v"; done
if command -v gst-inspect-1.0.exe >/dev/null || command -v libgstreamer-1.0-0.dll >/dev/null; then
  fail "GStreamer is still on PATH"
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
profile=$work/profile
mkdir -p "$profile/Roaming" "$profile/Local"
# Sona is a native program: it wants Windows paths in its environment.
APPDATA=$(cygpath -w "$profile/Roaming")
LOCALAPPDATA=$(cygpath -w "$profile/Local")
export SONA_PRIVATE=1 APPDATA LOCALAPPDATA

# check_layout DIR: the files every artefact carries, and not the CDM.
check_layout() {
  local d=$1 what=$2 f
  for f in sona.exe sona.ico sona_cdm_shim.dll libgstreamer-1.0-0.dll libglib-2.0-0.dll \
    libexec/gstreamer-1.0/gst-plugin-scanner.exe licenses/LICENSE.txt licenses/THIRD_PARTY_NOTICES.md; do
    [[ -f $d/$f ]] || fail "$what has no $f"
  done
  for f in coreelements typefindfunctions playback app audioconvert audioresample volume equalizer audioparsers isomp4 libav wasapi2; do
    [[ -f $d/lib/gstreamer-1.0/libgst$f.dll ]] || fail "$what has no plugin $f"
  done
  if bad=$(find "$d" -iname '*widevinecdm*') && [[ -n $bad ]]; then fail "$what contains the Widevine CDM: $bad"; fi
}

# 1. The portable zip.
# 7z is a native program: it wants Windows paths.
7z x -bso0 -bsp0 -o"$(cygpath -w "$work/zip")" "$(cygpath -w "$out/Sona-$version-windows-x64.zip")"
dir=$work/zip/Sona
exe=$dir/sona.exe
check_layout "$dir" "the zip"
zipfiles=$(cd "$dir" && find . -type f | sort) # before anything runs from it
echo "smoke(windows): the zip has $(find "$dir" -type f | wc -l) files, $(du -sm "$dir" | cut -f1) MB; plugins: $(cd "$dir/lib/gstreamer-1.0" && echo libgst*.dll | sed 's/libgst//g; s/\.dll//g')"
got=$("$exe" --version) || fail "sona.exe --version failed"
[[ $got == "sona $version" ]] || fail "sona.exe --version printed '$got', want 'sona $version'"

# Starts, paints, and the CDM host child (this executable again) readies the
# audio stack: it loads the bundled GStreamer and scans the bundled plugins,
# with the environment internal/playback/gstbundle sets.
log=$profile/Local/Sona/State/sona.log
registry=$profile/Local/Sona/gstreamer-1.0/registry.bin
"$exe" >"$work/sona.out" 2>&1 &
pid=$!
for _ in $(seq 1 60); do
  grep -q '"startup: first paint"' "$log" 2>/dev/null && break
  kill -0 "$pid" 2>/dev/null || break
  sleep 1
done
painted=0
grep -q '"startup: first paint"' "$log" 2>/dev/null && painted=1
if [[ $painted != 1 ]]; then
  kill "$pid" 2>/dev/null || true
  cat "$work/sona.out" "$log" 2>/dev/null || true
  fail "sona.exe never painted"
fi
echo "smoke(windows): the zip's sona.exe reached first paint"
ready=0
for _ in $(seq 1 120); do
  grep -q 'playback: audio stack ready' "$log" 2>/dev/null && { ready=1; break; }
  grep -q 'playback: audio stack not prepared' "$log" 2>/dev/null && break
  kill -0 "$pid" 2>/dev/null || break
  sleep 1
done
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
if [[ $ready != 1 ]]; then
  cat "$work/sona.out" "$log" 2>/dev/null || true
  fail "the audio stack was not readied from the bundled GStreamer"
fi
{ grep -o 'audio stack ready.*' "$log" | head -n 1 | cut -c1-300; } || true
echo "smoke(windows): the bundled GStreamer's audio stack is ready"

# The registry lists the bundle's plugins and nothing from an installation.
[[ -f $registry ]] || fail "no plugin registry at $registry"
grep -aq libgstcoreelements "$registry" || fail "the plugin registry does not list the bundle's plugins"
if [[ -n ${GSTREAMER_ROOT:-} ]]; then
  root=$(cygpath -m "$GSTREAMER_ROOT")
  if grep -aiqF -e "$root" -e "$(cygpath -w "$root")" "$registry"; then
    fail "the plugin registry names the GStreamer installation at $root"
  fi
fi

# 2. playspike, beside sona.exe so it finds the same DLLs, plays previews to
# fakesink: the engine, the CDM host child, the bundled GStreamer's decoders
# and no Music User Token, no CDM, no licence. It sets up GStreamer itself
# (gstbundle): no GST_* variable here. A fresh profile, so the registry is
# built again by this run.
tools=$out/smoke-tools
if [[ -f $tools/playspike.exe ]]; then
  cp "$tools/playspike.exe" "$dir/playspike.exe"
  rm -rf "$profile/Local/Sona/gstreamer-1.0"
  mkdir "$work/cwd" # no .env to find
  (cd "$work/cwd" && timeout 600 "$dir/playspike.exe" -preview -sink fakesink -storefront us) >"$work/preview.out" 2>&1 || { cat "$work/preview.out"; fail "preview playback"; }
  cat "$work/preview.out"
  grep -q 'preview advancing' "$work/preview.out" || fail "the preview did not advance"
  grep -q 'next preview playing' "$work/preview.out" || fail "the next preview did not play"
  grep -q 'decoder=avdec_aac' "$work/preview.out" || fail "the bundled libav did not decode"
  echo "smoke(windows): previews play from the bundle (fakesink)"
  rm "$dir/playspike.exe"
else
  echo "smoke(windows): no smoke-tools/playspike.exe (package.sh without SONA_SMOKE_TOOLS=1): previews not played"
fi

# 3. The update archive: the files at the top level.
mkdir "$work/tar"
tar -C "$work/tar" -xzf "$out/sona-$version-windows-amd64.tar.gz"
check_layout "$work/tar" "the update archive"
if [[ $(cd "$work/tar" && find . -type f | sort) != "$zipfiles" ]]; then
  fail "the update archive and the zip hold different files"
fi
echo "smoke(windows): the update archive holds the zip's files"

# 4. The installer, silently, into a folder of the test's own.
setup=$out/Sona-$version-windows-x64-setup.exe
dest=$work/installed
"$setup" /S "/D=$(cygpath -w "$dest")"
for f in sona.exe sona.ico uninstall.exe; do
  [[ -f $dest/$f ]] || fail "installer left no $f"
done
check_layout "$dest" "the installation"
if [[ $(cd "$dest" && find . -type f ! -name uninstall.exe | sort) != "$zipfiles" ]]; then
  fail "the installation and the zip hold different files"
fi
[[ $("$dest/sona.exe" --version) == "sona $version" ]] || fail "installed sona.exe prints the wrong version"
for scheme in music itms; do
  reg query "HKCU\\Software\\Classes\\$scheme\\shell\\open\\command" >/dev/null || fail "no $scheme: URL handler"
done
reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Sona' >/dev/null || fail "not listed under Apps"

# 5. The uninstaller removes the folder and the keys. It copies itself to a
# temporary folder to delete its own, so wait for the folder to go.
"$dest/uninstall.exe" /S
for _ in $(seq 1 30); do
  [[ -e $dest/sona.exe ]] || break
  sleep 1
done
[[ ! -e $dest/sona.exe ]] || fail "uninstall left sona.exe"
for _ in $(seq 1 30); do
  [[ -e $dest ]] || break
  sleep 1
done
[[ ! -e $dest/lib && ! -e $dest/libexec ]] || fail "uninstall left the GStreamer folders"
for scheme in music itms; do
  ! reg query "HKCU\\Software\\Classes\\$scheme" >/dev/null 2>&1 || fail "uninstall left the $scheme: keys"
done
echo "smoke(windows): PASS"
