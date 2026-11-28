#!/usr/bin/env bash
# fuzz-targets.sh: prints `matrix=<JSON>`, the job matrix of fuzz.yml: one
# {"pkg","target","id"} object for every Go fuzz target (func FuzzXxx) of the
# repository, found per package with `go test -list '^Fuzz'`. A package with
# a new target needs nothing else: the workflow runs it from the next week's
# schedule. Appends cleanly to $GITHUB_OUTPUT; to list the targets by hand:
#   .github/ci/fuzz-targets.sh | sed 's/^matrix=//' | jq -r '.[] | .pkg + " " + .target'
#
# Only the packages that have a Fuzz function in a _test.go file are built
# (listing every package compiles every test binary). A package whose test
# binary does not build here (CGO_ENABLED=0), or whose targets the default
# build does not list (a build tag), fails the job: a target that never runs
# would otherwise pass unnoticed. Keep cgo-only tests out of a fuzzed
# package's default build with a `cgo` constraint.
set -euo pipefail
cd "$(dirname "$0")/../.."

dirs=$(git ls-files -z --cached --others --exclude-standard -- '*_test.go' | xargs -0 -r grep -lE '^func Fuzz[A-Z0-9_]' | xargs -r -n1 dirname | sort -u | sed 's#^#./#')
if [ -z "$dirs" ]; then
	echo "fuzz-targets.sh: no fuzz targets in the repository" >&2
	exit 1
fi

list=$(
	for dir in $dirs; do
		if ! out=$(go test -run '^$' -list '^Fuzz' "$dir" 2>&1); then
			printf '%s\n' "$out" >&2
			echo "::error::$dir: its test binary does not build without cgo, so its fuzz targets cannot run" >&2
			exit 1
		fi
		names=$(grep '^Fuzz' <<<"$out" || true)
		if [ -z "$names" ]; then
			echo "::error::$dir has a Fuzz function that the default build does not list (a build tag?)" >&2
			exit 1
		fi
		for name in $names; do
			echo "$dir $name"
		done
	done
)
if [ -z "$list" ]; then
	echo "fuzz-targets.sh: no fuzz target listed" >&2
	exit 1
fi

# id names the job's artifact: the package path and the target, with no slashes.
printf 'matrix='
jq -Rcn '[inputs | split(" ") | {pkg: .[0], target: .[1], id: ((.[0] | ltrimstr("./") | gsub("/"; "-")) + "-" + .[1])}]' <<<"$list"
