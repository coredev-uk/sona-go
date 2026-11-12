#!/usr/bin/env bash
# shard.sh POOL INDEX COUNT: prints the test units of shard INDEX (1-based) of
# COUNT in POOL, one per line. A unit is a package path (./internal/x) or
# ./internal/x@dev (that package's run with -tags dev).
#
# Pools split the packages by what they need:
#   playback  the packages that need the real CDM, GStreamer's plugins or user
#             namespaces: the real-CDM, sandbox, preview, pipeline and
#             motion-decoder tests (one job)
#   ui-race   the native UI's packages that start goroutines (store, kit,
#             lyricsync); a pull request runs them under -race
#   ui        the other native UI packages (internal/ui/..., their dev-tag runs)
#   ui-all    ui-race and ui together
#   core      everything else
# Units are packed greedily, the heaviest first onto the lightest shard, by the
# seconds in timings.txt (a unit missing there weighs DEFAULT_WEIGHT). A package
# too heavy for one shard is split by test: its units are <path>#<part>/<parts>
# (test.sh runs the part's share of the tests), weighing timings.txt's figure
# for the package divided by the parts.
set -euo pipefail
cd "$(dirname "$0")/../.."
pool=${1:?pool} idx=${2:?index} count=${3:?count}
here=.github/ci
playback=(./internal/playback/widevine ./internal/playback/cdmhost ./internal/diag ./internal/playback/native ./internal/playback/gst ./internal/ui/media)
split=(./internal/ui/kit:2)
uirace=(./internal/ui/store ./internal/ui/kit ./internal/ui/lyricsync)
dev=(./internal/devinstance ./internal/ui/devstate ./internal/ui/app ./internal/ui/features/song ./internal/app ./internal/mpris)

units() {
	local u p n i
	{
		go list -f '{{.ImportPath}}' ./... | sed 's|^github.com/coredev-uk/sona-go|.|'
		printf '%s@dev\n' "${dev[@]}"
	} | while read -r u; do
		n=1
		for p in "${split[@]}"; do [[ $u == "${p%:*}" ]] && n=${p#*:}; done
		if [[ $n -eq 1 ]]; then
			echo "$u"
		else
			for ((i = 1; i <= n; i++)); do echo "$u#$i/$n"; done
		fi
	done
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
		base=${base%%#*}
		case $base in ./e2e/* | ./tests/*) continue ;; esac
		if member "$base" "${playback[@]}"; then
			[[ $u != *@dev && $pool == playback ]] && echo "$u"
		elif [[ $base == ./internal/ui/* ]]; then
			case $pool in
			ui-all) echo "$u" ;;
			ui-race) member "$base" "${uirace[@]}" && echo "$u" ;;
			ui) member "$base" "${uirace[@]}" || echo "$u" ;;
			esac
		else
			[[ $pool == core ]] && echo "$u"
		fi
	done
	return 0
}
units | select_pool | awk -v idx="$idx" -v n="$count" -v def="${DEFAULT_WEIGHT:-2}" '
	FNR == NR { if ($1 !~ /^#/ && NF >= 2) w[$1] = $2; next }
	{
		u[++k] = $1
		if ($1 in w) x[k] = w[$1]
		else if (split($1, part, "#") == 2 && part[1] in w) { split(part[2], of, "/"); x[k] = w[part[1]] / of[2] }
		else x[k] = def
	}
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
