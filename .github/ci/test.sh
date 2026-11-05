#!/usr/bin/env bash
# test.sh POOL INDEX COUNT: runs shard INDEX of COUNT of POOL (shard.sh) under
# -race, each unit once, through gotestsum. The JSON of each run is kept in
# $RESULTS_DIR (default $RUNNER_TEMP/test-results) for skips.sh and for
# timings.sh.
#
#   TEST_FLAGS  extra go test flags (e.g. -count=1)
#   ARTIFACT_DIR  where a failed golden leaves its picture and diff
#               (exported as SONA_TEST_ARTIFACTS, read by uitest/artifacts)
set -euo pipefail
cd "$(dirname "$0")/../.."
pool=${1:?pool} idx=${2:?index} count=${3:?count}
results=${RESULTS_DIR:-${RUNNER_TEMP:-/tmp}/test-results}
artifacts=${ARTIFACT_DIR:-${RUNNER_TEMP:-/tmp}/ui-artifacts}
mkdir -p "$results" "$artifacts"
export SONA_TEST_ARTIFACTS=$artifacts

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

# run NAME GO-TEST-ARGS... -- PACKAGES...: one gotestsum run. A failing test
# is run again up to twice (gotestsum swaps its own -run in, so the parts below
# still work); one that passes on a rerun passes the job and is reported as
# flaky. The JSON of every attempt stays in $results for skips.sh and
# timings.sh. -artifacts is not passed: go test never caches a run that has it
# (uitest/artifacts reads $SONA_TEST_ARTIFACTS instead).
run() {
	local name=$1 flags=() pkgs=()
	shift
	while [[ $1 != -- ]]; do
		flags+=("$1")
		shift
	done
	shift
	pkgs=("$@")
	local report="$results/$name.rerun" st=0
	rm -f "$report"
	# shellcheck disable=SC2086
	go tool gotestsum --jsonfile "$results/$name.json" --format pkgname-and-test-fails \
		--rerun-fails=2 --rerun-fails-report="$report" --packages "${pkgs[*]}" \
		-- -race ${TEST_FLAGS:-} ${flags[@]+"${flags[@]}"} || st=$?
	if [[ $st -eq 0 && -s $report ]]; then
		echo "::warning title=Flaky tests::$name: failed, then passed on a rerun: $(tr '\n' ';' <"$report")"
		if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
			{
				echo "### Flaky tests ($name)"
				echo
				echo "Each failed, then passed on a rerun:"
				echo
				sed 's/^/- /' "$report"
			} >>"$GITHUB_STEP_SUMMARY"
		fi
	fi
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
	(run "$pool-$idx-part$(basename "$pkg")-$i" -run "$(part "$pkg" "$i" "$n")" -- "$pkg") &
	bg+=($!)
done
[[ ${#prod[@]} -eq 0 ]] || run "$pool-$idx" -- "${prod[@]}" || status=1
[[ ${#dev[@]} -eq 0 ]] || run "$pool-$idx-dev" -tags dev,sqlite_fts5 -- "${dev[@]}" || status=1
for pid in ${bg[@]+"${bg[@]}"}; do wait "$pid" || status=1; done
exit "$status"
