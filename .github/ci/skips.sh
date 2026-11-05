#!/usr/bin/env bash
# skips.sh RESULTS_DIR...: fails on every skipped test (subtests included) in
# the go test -json files that the allow-list does not name. A skip is a
# missing prerequisite or a test nobody runs; the ones allowed are those that
# cannot run on the platform (live tests, evidence writers, Linux-only
# goldens). The list is $SKIPS_FILE, default .github/ci/skips-linux.txt or
# skips-darwin.txt by `uname`. Lines:
#
#   <package suffix | *>  <test regexp>  [reason substring]
#
# The regexp is anchored and matches the top-level test's name (no spaces; use
# `|`); the reason, which may hold spaces, must occur in the skip's log lines.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
default=skips-linux.txt
[[ $(uname -s) == Darwin ]] && default=skips-darwin.txt
list=${SKIPS_FILE:-$here/$default}
files=()
for d in "$@"; do files+=("$d"/*.json); done

allow=$(jq -R -s '
	split("\n") | map(select(length > 0 and (startswith("#") | not))
		| capture("^(?<pkg>\\S+)\\s+(?<re>\\S+)(\\s+(?<reason>.+))?$")
		// error("bad line in the allow-list"))' "$list")

# One line per unallowed skip: package, test, first log line that is not
# bookkeeping.
bad=$(jq -s -r --argjson allow "$allow" '
	[.[] | select(.Test != null)] as $ev
	| (reduce $ev[] as $e ({}; if $e.Action == "output" then .[$e.Package + "|" + $e.Test] += $e.Output else . end)) as $out
	| $ev[] | select(.Action == "skip")
	| . as $s
	| ($out[$s.Package + "|" + $s.Test] // "") as $log
	| select(any($allow[]; . as $a
		| ($a.pkg == "*" or ($s.Package | endswith($a.pkg)))
		and ($s.Test | split("/")[0] | test("^(" + $a.re + ")$"))
		and ($a.reason == null or ($log | contains($a.reason)))) | not)
	| "\($s.Package | sub("^github.com/coredev-uk/sona-go/"; "")) \($s.Test): \($log | split("\n") | map(select(test("^\\s+\\S+:\\d+: "))) | first // "no reason")"
' "${files[@]}" | sort -u)

if [[ -n $bad ]]; then
	while read -r line; do echo "::error::skipped, not in $(basename "$list"): $line"; done <<<"$bad"
	exit 1
fi
echo "skips: every skipped test is in $(basename "$list")"
