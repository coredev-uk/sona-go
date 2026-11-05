#!/usr/bin/env bash
# find-image.sh: waits for the CI image of this checkout's Containerfile and
# go.mod (build/ci/tag.sh) and appends `ref=<image>@<digest>` and `id=<digest>`
# to $GITHUB_OUTPUT. A tag no run has built yet is being built by the Images
# workflow (30 minutes at most); an existing one is there at once. Needs
# docker, logged in to ghcr.io by the caller when the package is private
# (GH_TOKEN and ACTOR do it here).
set -euo pipefail
cd "$(dirname "$0")/../.."
tag=$(build/ci/tag.sh)
image=ghcr.io/coredev-uk/sona-ci
printf '%s' "${GH_TOKEN:?}" | docker login ghcr.io -u "${ACTOR:?}" --password-stdin
for _ in $(seq 1 90); do
	if digest=$(docker buildx imagetools inspect "$image:$tag" --format '{{.Manifest.Digest}}' 2>/dev/null) && [ -n "$digest" ]; then
		{
			echo "ref=$image@$digest"
			echo "id=${digest#sha256:}"
		} >>"${GITHUB_OUTPUT:?}"
		echo "CI image $image:$tag is $digest"
		exit 0
	fi
	echo "waiting for $image:$tag (Images workflow) …"
	sleep 20
done
echo "::error::$image:$tag does not exist: run the Images workflow (Actions → Images → Run workflow); a fork's pull request that changes build/ci can't publish it"
exit 1
