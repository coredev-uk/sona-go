#!/usr/bin/env bash
# find-image.sh: waits for the CI image of this checkout's Containerfile and
# go.mod (build/ci/tag.sh) and appends `ref=<image>@<digest>` and `id=<digest>`
# to $GITHUB_OUTPUT. A tag no run has built yet is being built by the Images
# workflow (30 minutes at most); an existing one is there at once. Needs
# docker and gh, logged in to ghcr.io by the caller when the package is
# private (GH_TOKEN and ACTOR do it here).
#
# Which image counts depends on who is asking (the Images workflow pushes
# <tag> only from main, and a pull request's build under pr-<tag>):
#   - a pull request run (GITHUB_EVENT_NAME=pull_request) takes <tag>, else the
#     pr-<tag> its own change built. Its jobs hold no secret, and its caches
#     are scoped to its ref.
#   - every other run (main, the nightly, a release) takes only an <tag> whose
#     build provenance says images.yml built it on refs/heads/main
#     (gh attestation verify), so a branch's run can't substitute an image the
#     main-scope runs would then execute.
#
#   build/ci/find-image.sh            wait, then write the outputs
#   build/ci/find-image.sh --check    one attempt: exit 0 when an image is usable
set -euo pipefail
cd "$(dirname "$0")/../.."
tag=$(build/ci/tag.sh)
image=ghcr.io/coredev-uk/sona-ci
repo=${GITHUB_REPOSITORY:-coredev-uk/sona-go}
printf '%s' "${GH_TOKEN:?}" | docker login ghcr.io -u "${ACTOR:?}" --password-stdin

digest_of() { docker buildx imagetools inspect "$image:$1" --format '{{.Manifest.Digest}}' 2>/dev/null; }

# Prints the digest of the usable image, or nothing.
find_digest() {
	local d
	if [ "${GITHUB_EVENT_NAME:-}" = pull_request ]; then
		for t in "$tag" "pr-$tag"; do
			if d=$(digest_of "$t") && [ -n "$d" ]; then
				echo "$d"
				return 0
			fi
		done
		return 0
	fi
	d=$(digest_of "$tag") || return 0
	[ -n "$d" ] || return 0
	if gh attestation verify "oci://$image@$d" -R "$repo" \
		--signer-workflow "$repo/.github/workflows/images.yml" \
		--source-ref refs/heads/main >/tmp/find-image-verify.log 2>&1; then
		echo "$d"
	else
		echo "sona-ci:$tag ($d) has no build provenance from images.yml on main yet" >&2
		tail -n 5 /tmp/find-image-verify.log >&2 || true
	fi
}

if [ "${1:-}" = --check ]; then
	[ -n "$(find_digest)" ]
	exit
fi

for _ in $(seq 1 90); do
	digest=$(find_digest)
	if [ -n "$digest" ]; then
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
echo "::error::$image:$tag does not exist or has no provenance from main: run the Images workflow (Actions → Images → Run workflow); a fork's pull request that changes build/ci can't publish it"
exit 1
