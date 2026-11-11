#!/usr/bin/env bash
# Fetches the official GStreamer Windows packages (MinGW, x86_64: runtime and
# development files in one installer), checks the installer against
# gstreamer.sha256, installs it without a prompt and makes its pkg-config
# files usable where they are (run in Git Bash on windows-2025).
#
#   build/windows/install-gstreamer.sh --fetch     # only download and check the installer
#   build/windows/install-gstreamer.sh [DEST]      # also install it
#
# DEST is where it installs (default: $RUNNER_TEMP/gstreamer). The last two
# lines on stdout are what a build needs, as KEY=VALUE:
#
#   GSTREAMER_ROOT=<the installation: bin/, lib/, libexec/, in C:/… form>
#   PKG_CONFIG_PATH=<pkg-config files whose prefix is GSTREAMER_ROOT>
#
# The pinned version and the installer's SHA-256 are in gstreamer.sha256;
# change both together (same 1.28 minor as build/darwin and devenv). The
# installer comes from this repository's build-deps mirror, else upstream
# (build/gstreamer-fetch.sh), and is cached in $GSTREAMER_PKG_CACHE (default:
# $RUNNER_TEMP/gstreamer-pkg).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=build/gstreamer-fetch.sh
. "$here/../gstreamer-fetch.sh"
version=$(awk '$1=="version"{print $2}' "$here/gstreamer.sha256")
name=gstreamer-1.0-mingw-x86_64-$version.exe
tmp=${RUNNER_TEMP:-${TEMP:-/tmp}}
cache=${GSTREAMER_PKG_CACHE:-$tmp/gstreamer-pkg}
fetch=
[[ ${1:-} == --fetch ]] && { fetch=1; shift; }
dest=${1:-$tmp/gstreamer}
mkdir -p "$cache"

if [[ ! -f $cache/$name ]]; then
  gstreamer_fetch "$name" "https://gstreamer.freedesktop.org/data/pkg/windows/$version/mingw/$name" "$cache/$name" || exit 1
fi
want=$(awk -v n="$name" '$2==n{print $1}' "$here/gstreamer.sha256")
# From stdin: sha256sum puts a backslash before the hash when a file name has one.
got=$(sha256sum <"$cache/$name" | awk '{print $1}')
[[ -n $want && $got == "$want" ]] || { echo "install-gstreamer: $name sha256 $got, want ${want:-<none>}" >&2; exit 1; }

[[ -z $fetch ]] || exit 0

rm -rf "$dest"
mkdir -p "$dest"
# MSYS2_ARG_CONV_EXCL: Git Bash would turn /VERYSILENT into a path.
log=$cache/install.log
if ! MSYS2_ARG_CONV_EXCL='*' "$cache/$name" /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /CURRENTUSER \
  "/DIR=$(cygpath -w "$dest")" "/LOG=$(cygpath -w "$log")" >&2; then
  echo "install-gstreamer: the installer failed; its log:" >&2
  tail -n 60 "$log" >&2 || true
  exit 1
fi

# The installation's root is where bin/ holds the core library.
dll=$(find "$dest" -type f -name 'libgstreamer-1.0-0.dll' -path '*/bin/*' | head -n 1)
[[ -n $dll ]] || { echo "install-gstreamer: no libgstreamer-1.0-0.dll under $dest" >&2; find "$dest" -maxdepth 3 >&2; exit 1; }
root=$(dirname "$(dirname "$dll")")
rootm=$(cygpath -m "$root")
[[ -f $root/lib/pkgconfig/gstreamer-1.0.pc ]] || { echo "install-gstreamer: the installation has no development files ($root/lib/pkgconfig)" >&2; find "$root/lib" -maxdepth 1 >&2; exit 1; }

# GStreamer's lib/ also holds the MinGW runtime's own libraries (libgcc.a,
# libmingw32.a, libmingwex.a, libmoldname.a, …), from a different toolchain
# than the compiler cgo uses. A -L for lib/ puts them ahead of the
# compiler's, and the program then crashes at start ("32 bit pseudo
# relocation … out of range" in crt2's call to exit). So the link searches
# a directory of copies of the import libraries (libglib-2.0.dll.a, …) and
# nothing else.
libs=$dest/sona-lib
mkdir -p "$libs"
for f in "$root"/lib/*.dll.a; do
  case $(basename "$f") in
    libwinpthread* | libgcc* | libmingw* | libmoldname* | libmsvcrt* | libucrt* | libstdc++*) continue ;;
  esac
  cp "$f" "$libs/"
done
libsm=$(cygpath -m "$libs")

# The .pc files name their prefix as ${pcfiledir}/../.. (which only pkgconf
# resolves) or as the default install location: point both at the root, and
# the libraries' -L at the directory above.
pc=$dest/sona-pkgconfig
mkdir -p "$pc"
for f in "$root"/lib/pkgconfig/*.pc; do
  sed -e "s|\${pcfiledir}/../..|$rootm|g" -e "s|^prefix=.*|prefix=$rootm|" \
    -e '/^Libs/ s|-L\${libdir}|-L'"$libsm"'|g' -e '/^Libs/ s|-L\${prefix}/lib|-L'"$libsm"'|g' "$f" >"$pc/$(basename "$f")"
done
echo "install-gstreamer: GStreamer $version in $rootm" >&2
echo "GSTREAMER_ROOT=$rootm"
echo "PKG_CONFIG_PATH=$(cygpath -m "$pc")"
