#!/usr/bin/env bash
# Builds sona_cdm_shim.dll, Sona's Chromium CDM host shim
# (internal/playback/widevine/shim.cc), with MSVC (run in Git Bash on
# windows-2025, whose Visual Studio has the C++ tools).
#
#   build/windows/build-shim.sh OUTDIR
#
# Why a separate DLL: Google's widevinecdm.dll is built for the MSVC C++ ABI
# and the host side of its interface is C++ classes with virtual methods,
# while cgo compiles with MinGW, whose ABI differs. The DLL exports the plain
# C API of shim.h; cgo's side (shim_windows.c) calls it through function
# pointers. The C runtime is linked statically (/MT), so the DLL needs no
# Visual C++ redistributable and shares no heap with Go or GStreamer.
#
# Prints the DLL's exports and its dependencies, and fails if it imports
# anything but Windows' own libraries or lacks an export of shim.h.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
out=${1:?usage: build-shim.sh OUTDIR}
mkdir -p "$out"
out=$(cd "$out" && pwd)
src=$repo/internal/playback/widevine

vswhere="/c/Program Files (x86)/Microsoft Visual Studio/Installer/vswhere.exe"
[[ -x $vswhere ]] || { echo "build-shim: vswhere.exe not found: Visual Studio is not installed" >&2; exit 1; }
vs=$("$vswhere" -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath | tr -d '\r')
[[ -n $vs ]] || { echo "build-shim: no Visual Studio with the C++ x64 tools" >&2; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
w() { cygpath -w "$1"; }

cat >"$work/build.bat" <<EOF
@echo off
call "$(w "$vs/VC/Auxiliary/Build/vcvars64.bat")" >nul || exit /b 1
cl /nologo /std:c++17 /O2 /MT /EHsc /W3 /DSONA_SHIM_BUILD /D_CRT_SECURE_NO_WARNINGS /LD ^
  /I"$(w "$src")" /Fo"$(w "$work")\shim.obj" /Fe"$(w "$out/sona_cdm_shim.dll")" "$(w "$src/shim.cc")" ^
  /link /INCREMENTAL:NO || exit /b 1
echo --- exports
dumpbin /nologo /exports "$(w "$out/sona_cdm_shim.dll")" || exit /b 1
echo --- dependents
dumpbin /nologo /dependents "$(w "$out/sona_cdm_shim.dll")" || exit /b 1
EOF
cmd.exe //c "$(w "$work/build.bat")" | tee "$work/log"

# Every function of shim.h is exported.
for f in $(sed -n 's/^SONA_API [^(]* \**\(sona_[a-z_0-9]*\)(.*/\1/p' "$src/shim.h"); do
  grep -q "\b$f\b" "$work/log" || { echo "build-shim: sona_cdm_shim.dll does not export $f" >&2; exit 1; }
done
# Only Windows' own DLLs: no Visual C++ runtime.
deps=$(sed -n '/^--- dependents/,$p' "$work/log" | grep -i '\.dll' | tr -d '\r ' | tr 'A-Z' 'a-z' | grep -v '^sona_cdm_shim' || true)
echo "$deps" | grep -Eq 'vcruntime|msvcp|ucrtbase' && { echo "build-shim: the DLL depends on the Visual C++ runtime: $deps" >&2; exit 1; }
ls -l "$out/sona_cdm_shim.dll"
