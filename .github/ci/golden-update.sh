#!/usr/bin/env bash
# golden-update.sh: rewrites the golden images of the UI tests that fail, in
# the environment it runs in (CI's image is the reference: the goldens depend
# on its Pango and fonts). A package's tests run once as they are; the failing
# top-level tests run again with -update, so images that still match are not
# rewritten. golden-update.yml runs it and commits what changed.
set -euo pipefail
cd "$(dirname "$0")/../.."
export MYGO_GPU=0
pkgs=$(find internal/ui -type d -path '*/testdata/golden' | sed 's|/testdata/golden$||' | sort -u)
for p in $pkgs; do
	fails=$(go test -count=1 -json "./$p" 2>/dev/null | jq -r 'select(.Action == "fail" and .Test != null and (.Test | contains("/") | not)) | .Test' | sort -u | paste -sd'|' - || true)
	[[ -n $fails ]] || continue
	echo "$p: updating $fails"
	flags=(-update)
	[[ $p == internal/ui/kit ]] && flags+=(-update-controls -update-media)
	go test -count=1 "./$p" -run "^($fails)\$" "${flags[@]}" >/dev/null || true
done
git status --short -- 'internal/ui/**/testdata/golden/*.png'
