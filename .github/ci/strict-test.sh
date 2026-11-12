#!/usr/bin/env bash
# strict-test.sh LOG [OPTIONS] -- go test ARGS...: runs `go test` (use -v) with
# its output in LOG, prints the log without the RUN/PAUSE/CONT lines (each cut
# to 400 columns) and fails unless the tests passed the way the options ask. For
# jobs where a skipped test means a missing prerequisite (a runner with no Media
# Foundation, no AppContainers), not a pass.
#
#   --lines N         print at most N lines of the log (default 250)
#   --pass TEST       TEST must have passed (repeatable)
#   --skips           any skipped test fails
#   --allow-skip RE   a skipped test fails unless its line matches RE (implies --skips)
set -uo pipefail
log=${1:?log}
shift
lines=250 pass=() skips='' allow=''
while (($#)); do
  case $1 in
    --lines) lines=${2:?--lines needs a number}; shift 2 ;;
    --pass) pass+=("${2:?--pass needs a test}"); shift 2 ;;
    --skips) skips=1; shift ;;
    --allow-skip) skips=1 allow=${2:?--allow-skip needs a pattern}; shift 2 ;;
    --) shift; break ;;
    *) echo "strict-test.sh: unknown argument $1" >&2; exit 2 ;;
  esac
done
"$@" >"$log" 2>&1
rc=$?
grep -vE '^=== (RUN|PAUSE|CONT)' "$log" | cut -c1-400 | head -n "$lines"
for t in ${pass[@]+"${pass[@]}"}; do
  grep -q -- "--- PASS: $t " "$log" || { echo "$t did not pass"; rc=1; }
done
if [[ -n $skips ]]; then
  bad=$(grep -- '--- SKIP' "$log")
  [[ -z $allow || -z $bad ]] || bad=$(grep -vE -- "$allow" <<<"$bad")
  if [[ -n $bad ]]; then echo "$bad"; echo "a test was skipped"; rc=1; fi
fi
exit "$rc"
