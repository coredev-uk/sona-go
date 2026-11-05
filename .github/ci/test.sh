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
prod=() dev=() parts=()
while read -r u; do
	[[ -n $u ]] || continue
	case $u in
	*@dev) dev+=("${u%@dev}") ;;
	*#*) parts+=("$u") ;;
	*) prod+=("$u") ;;
	esac
done <<<"$units"
echo "shard $pool $idx/$count: ${#prod[@]} packages, ${#parts[@]} part-packages, ${#dev[@]} with -tags dev"
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

# part PKG I N: the -run pattern of part I of N of PKG's tests (every N-th
# test, in the order go test -list prints them).
part() {
	go test -race -list . "$1" | awk -v i="$2" -v n="$3" '/^(Test|Example|Fuzz)[A-Za-z0-9_]*$/ { if (k++ % n == i - 1) names = names (names ? "|" : "") $0 } END { print "^(" names ")$" }'
}

status=0
bg=()
# The parts of a package run beside the rest: one core's worth of tests each.
for u in ${parts[@]+"${parts[@]}"}; do
	pkg=${u%%#*} frac=${u#*#}
	i=${frac%/*} n=${frac#*/}
	(run "$pool-$idx-part$(basename "$pkg")-$i" -run "$(part "$pkg" "$i" "$n")" "$pkg") &
	bg+=($!)
done
[[ ${#prod[@]} -eq 0 ]] || run "$pool-$idx" "${prod[@]}" || status=1
[[ ${#dev[@]} -eq 0 ]] || run "$pool-$idx-dev" -tags dev,sqlite_fts5 "${dev[@]}" || status=1
for pid in ${bg[@]+"${bg[@]}"}; do wait "$pid" || status=1; done
exit "$status"
