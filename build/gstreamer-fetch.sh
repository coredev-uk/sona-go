# shellcheck shell=bash
# Sourced by build/windows/install-gstreamer.sh and build/darwin/install-gstreamer.sh:
# gstreamer_fetch NAME UPSTREAM_URL OUT downloads one pinned GStreamer package to OUT.
#
# It tries this repository's mirror first: the `build-deps` prerelease, which
# .github/workflows/build-deps.yml fills (from the Actions cache, else
# gstreamer.freedesktop.org) with the packages the same gstreamer.sha256 files
# pin. GitHub serves it in seconds to a runner; freedesktop.org is slow for
# everyone, sits behind a bot check, and has taken most of an hour for the
# Windows installer (about 1 GB) or ended every attempt early (curl 18, release
# runs 42606615741 and 43014335716). Upstream stays the
# fallback, for a version not mirrored yet; GSTREAMER_MIRROR set empty skips the
# mirror. The caller checks the SHA-256 of whatever arrived, so the mirror is
# trusted no more than upstream.
GSTREAMER_MIRROR=${GSTREAMER_MIRROR-https://github.com/coredev-uk/sona-go/releases/download/build-deps}

gstreamer_fetch() {
  local name=$1 upstream=$2 out=$3 part=$3.part attempt
  if [[ -n $GSTREAMER_MIRROR ]] &&
    curl -fsSL --retry 5 --retry-all-errors --connect-timeout 30 -o "$part" "$GSTREAMER_MIRROR/$name" >&2; then
    mv "$part" "$out"
    echo "install-gstreamer: $name from the mirror" >&2
    return 0
  fi
  rm -f "$part"
  echo "install-gstreamer: $name not on the mirror; from $upstream" >&2
  # freedesktop.org resets or ends responses mid-download (curl 92 and 18),
  # and its HTTP/2 resets streams: each attempt resumes where the last
  # stopped (-C -) into a .part file, over HTTP/1.1. GSTREAMER_ATTEMPTS (30)
  # bounds them; the mirror job, with hours to spare, allows more.
  for attempt in $(seq 1 "${GSTREAMER_ATTEMPTS:-30}"); do
    if curl -fsSL --http1.1 -C - --connect-timeout 30 -o "$part" "$upstream" >&2; then
      mv "$part" "$out"
      return 0
    fi
    echo "install-gstreamer: download attempt $attempt stopped at $(wc -c <"$part" 2>/dev/null || echo 0) bytes; resuming" >&2
    sleep 5
  done
  echo "install-gstreamer: $name did not download in ${GSTREAMER_ATTEMPTS:-30} attempts" >&2
  return 1
}
