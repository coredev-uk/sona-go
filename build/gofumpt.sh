#!/bin/sh
# gofumpt over every tracked Go file except internal/db/dbq (sqlc's output),
# with the version go.mod pins (`go tool gofumpt`), so a machine, `check`, `fmt`
# and CI (.github/workflows/ci.yml, "Vet and type-check") agree on one
# formatting (#409). One definition of the file list, run from any directory:
#
#   build/gofumpt.sh        lists the unformatted files and fails if there are any
#   build/gofumpt.sh -w     formats them
#
# xargs keeps each command line under the system's limit (Windows' is 32 KiB).
set -eu
cd "$(dirname "$0")/.."
case ${1:-check} in
check)
	unformatted=$(git ls-files '*.go' | grep -v '^internal/db/dbq/' | xargs go tool gofumpt -l)
	if [ -n "$unformatted" ]; then
		echo "gofumpt: not formatted (run fmt, or build/gofumpt.sh -w):" >&2
		echo "$unformatted" >&2
		exit 1
	fi
	;;
-w) git ls-files '*.go' | grep -v '^internal/db/dbq/' | xargs go tool gofumpt -w ;;
*)
	echo "usage: gofumpt.sh [-w]" >&2
	exit 2
	;;
esac
