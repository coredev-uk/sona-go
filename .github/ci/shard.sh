#!/usr/bin/env bash
# shard.sh POOL INDEX COUNT: prints the test units of shard INDEX (1-based) of
# COUNT in POOL, one per line. A unit is a package path (./internal/x) or
# ./internal/x@dev (that package's run with -tags dev).
#
# Pools split the packages by what they need:
#   playback  the real-CDM, sandbox, preview and motion-decoder packages (one job)
#   ui        the native UI's packages (internal/ui/..., their dev-tag runs)
#   core      everything else
# Units are packed greedily, the heaviest first onto the lightest shard, by the
# seconds in timings.txt (a unit missing there weighs DEFAULT_WEIGHT).
set -euo pipefail
cd "$(dirname "$0")/../.."
pool=${1:?pool} idx=${2:?index} count=${3:?count}
here=.github/ci
playback=(./internal/playback/widevine ./internal/playback/cdmhost ./internal/diag ./internal/playback/native ./internal/ui/media)
dev=(./internal/devinstance ./internal/ui/devstate ./internal/ui/app ./internal/app ./internal/mpris)

units() {
	go list -f '{{.ImportPath}}' ./... | sed 's|^github.com/coredev-uk/sona-go|.|'
	printf '%s@dev\n' "${dev[@]}"
}
member() { # member NAME LIST...
	local n=$1 x
	shift
	for x in "$@"; do [[ $x == "$n" ]] && return 0; done
	return 1
}
select_pool() {
	local u base
	while read -r u; do
		base=${u%@dev}
		case $base in ./e2e/* | ./tests/*) continue ;; esac
		if member "$base" "${playback[@]}"; then
			[[ $u == "$base" && $pool == playback ]] && echo "$u"
		elif [[ $base == ./internal/ui/* ]]; then
			[[ $pool == ui ]] && echo "$u"
		else
			[[ $pool == core ]] && echo "$u"
		fi
	done
	return 0
}
units | select_pool | awk -v idx="$idx" -v n="$count" -v def="${DEFAULT_WEIGHT:-2}" '
	FNR == NR { if ($1 !~ /^#/ && NF >= 2) w[$1] = $2; next }
	{ u[++k] = $1; x[k] = ($1 in w) ? w[$1] : def }
	END {
		for (i = 1; i <= k; i++) ord[i] = i
		for (i = 2; i <= k; i++) { # insertion sort, heaviest first, name for ties
			v = ord[i]
			for (j = i - 1; j >= 1 && (x[ord[j]] < x[v] || (x[ord[j]] == x[v] && u[ord[j]] > u[v])); j--) ord[j + 1] = ord[j]
			ord[j + 1] = v
		}
		for (s = 1; s <= n; s++) load[s] = 0
		for (i = 1; i <= k; i++) {
			b = 1
			for (s = 2; s <= n; s++) if (load[s] < load[b]) b = s
			load[b] += x[ord[i]]
			if (b == idx) print u[ord[i]]
		}
	}' "$here/timings.txt" -
