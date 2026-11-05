#!/usr/bin/env bash
# mustpass.sh RESULTS_DIR...: fails unless every line of mustpass.txt whose
# package ran in the given go test -json files matched at least one passing
# test and no skipped or failed one.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
files=()
for d in "$@"; do files+=("$d"/*.json); done
status=0
while read -r pkg re; do
	[[ -z $pkg || $pkg == \#* ]] && continue
	out=$(jq -s -r --arg pkg "$pkg" --arg re "^($re)\$" '
		[.[] | select(.Test != null and (.Package | endswith($pkg)) and (.Action == "pass" or .Action == "skip" or .Action == "fail"))] as $ev
		| [$ev[] | select(.Test | split("/")[0] | test($re))] as $m
		| ([$m[] | select(.Test | contains("/") | not) | select(.Action == "pass")] | length) as $passed
		| [$m[] | select(.Action != "pass") | "\(.Action) \(.Test)"] as $bad
		| if $passed == 0 then "no passing test matches" elif ($bad | length) > 0 then "not passing: " + ($bad | join(", ")) else "ok \($passed)" end
	' "${files[@]}")
	if [[ $out == ok* ]]; then
		echo "mustpass: $pkg /$re/: $out"
	else
		echo "::error::mustpass: $pkg /$re/: $out"
		status=1
	fi
done <"${MUSTPASS_FILE:-$here/mustpass.txt}"
exit "$status"
