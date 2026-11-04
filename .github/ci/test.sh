#!/usr/bin/env bash
# test.sh POOL INDEX COUNT: runs shard INDEX of COUNT of POOL (shard.sh) under
# -race, each unit once. The JSON of each go test run is kept in
# $RESULTS_DIR (default $RUNNER_TEMP/test-results) for mustpass.sh and for
# timings.sh; the log shows the plain output.
#
#   TEST_FLAGS  extra go test flags (e.g. -count=1)
#   -artifacts: a failed golden leaves its picture and diff in $ARTIFACT_DIR
set -euo pipefail
cd "$(dirname "$0")/../.."
pool=${1:?pool} idx=${2:?index} count=${3:?count}
results=${RESULTS_DIR:-${RUNNER_TEMP:-/tmp}/test-results}
artifacts=${ARTIFACT_DIR:-${RUNNER_TEMP:-/tmp}/ui-artifacts}
mkdir -p "$results" "$artifacts"

# bash 3.2 (macOS) has no mapfile and rejects an empty array under set -u.
units=$(.github/ci/shard.sh "$pool" "$idx" "$count")
prod=() dev=()
while read -r u; do
	[[ -n $u ]] || continue
	if [[ $u == *@dev ]]; then dev+=("${u%@dev}"); else prod+=("$u"); fi
done <<<"$units"
echo "shard $pool $idx/$count: ${#prod[@]} packages, ${#dev[@]} with -tags dev"
echo "$units" | sed 's/^/  /'

plain='(fromjson? // {Action:"output",Output:(.+"\n")}) | select(.Action=="output") | .Output'
run() { # run NAME GO-TEST-ARGS...: one go test, JSON kept, plain log
	local name=$1
	shift
	set +e
	# shellcheck disable=SC2086
	go test -race -json -artifacts -outputdir "$artifacts" ${TEST_FLAGS:-} "$@" | tee "$results/$name.json" | jq -R -rj --unbuffered "$plain"
	local st=${PIPESTATUS[0]}
	set -e
	return "$st"
}
status=0
[[ ${#prod[@]} -eq 0 ]] || run "$pool-$idx" "${prod[@]}" || status=1
[[ ${#dev[@]} -eq 0 ]] || run "$pool-$idx-dev" -tags dev,sqlite_fts5 "${dev[@]}" || status=1
exit "$status"
