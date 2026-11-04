#!/usr/bin/env bash
# timings.sh RESULTS_DIR...: prints timings.txt lines (unit, seconds) from the
# go test -json files of a run (the test-results-* artifacts: `gh run download
# <id> -p 'test-results-*'`), heaviest first. Files named *-dev.json are the
# -tags dev runs. Packages served from the test cache are skipped.
set -euo pipefail
for d in "$@"; do
	for f in "$d"/*.json; do
		suffix=
		[[ $f == *-dev.json ]] && suffix=@dev
		jq -R -r --arg s "$suffix" 'fromjson? | select(.Test == null and (.Action == "pass" or .Action == "fail") and .Elapsed != null) | "\(.Package | sub("^github.com/coredev-uk/sona-go"; ".") + $s) \(.Elapsed)"' "$f"
	done
done | sort -k2 -rn
