#!/usr/bin/env bash
# tag.sh: prints the image tag (and the build arguments) for what the
# Containerfile and go.mod say: the first 16 hex digits of the hash of the
# Containerfile, the Go version and gotestsum's version, so a change to any of
# them is a new image. `tag.sh args` prints the --build-arg flags for podman.
set -euo pipefail
cd "$(dirname "$0")/../.."
go_version=$(awk '$1 == "go" { print $2; exit }' go.mod)
gotestsum=$(awk '$1 == "gotest.tools/gotestsum" { print $2; exit }' go.mod)
[[ -n $go_version && -n $gotestsum ]] || {
	echo "tag.sh: go.mod names no go version or no gotestsum" >&2
	exit 1
}
case ${1:-tag} in
tag) { cat build/ci/Containerfile; echo "$go_version $gotestsum"; } | sha256sum | cut -c1-16 ;;
args) echo "--build-arg GO_VERSION=$go_version --build-arg GOTESTSUM_VERSION=$gotestsum" ;;
*) echo "usage: tag.sh [tag|args]" >&2; exit 2 ;;
esac
