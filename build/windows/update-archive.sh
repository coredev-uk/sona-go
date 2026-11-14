#!/usr/bin/env bash
# Writes the Windows update archive: a gzip tar of what is in DIR, every entry
# named from DIR's top level (sona.exe, lib/..., not ./sona.exe), with no
# entry for DIR itself.
#
#   build/windows/update-archive.sh DIR OUT.tar.gz
#
# MyGo's updater refuses an archive whole at the first entry whose name, cleaned,
# is "." (internal/update CheckArchive says what else it refuses): the "./" that
# `tar -C DIR -czf OUT .` writes first made every Windows update fail with
# `invalid path in update archive: "./"`. So the entries are listed by name,
# dotfiles included. package.sh runs this, internal/update's tests run it on a
# folder of their own and check the archive the way the updater would, and
# cmd/releasesign checks again before it signs.
set -euo pipefail

dir=${1:?usage: update-archive.sh DIR OUT.tar.gz}
out=${2:?usage: update-archive.sh DIR OUT.tar.gz}

entries=()
while IFS= read -r name; do entries+=("$name"); done < <(cd "$dir" && ls -A)
((${#entries[@]})) || { echo "update-archive: $dir is empty" >&2; exit 1; }
tar -C "$dir" -czf "$out" -- "${entries[@]}"
