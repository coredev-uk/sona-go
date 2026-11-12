#!/usr/bin/env bash
# Prints the -ldflags of a production build of Sona, for `go build -ldflags
# "$(build/ldflags.sh ...)"`: the Taskfiles, devenv's build, bundle.sh,
# package.sh and CI's build steps share it. (The Flatpak and Nix builds are
# sandboxed and carry their own copy.)
#
#   build/ldflags.sh [--goos darwin|windows|linux] [--update TARGET] [--no-credentials]
#
# Always: -w -s, the version (VERSION, else the VERSION file) as main.version,
# and MyGo's production mode. --goos darwin adds the link's deployment target
# (MACOSX_DEPLOYMENT_TARGET, when set); windows adds -H windowsgui.
#
# From the environment, when set (builds without them read them at run time):
#   SONA_LASTFM_KEY, SONA_LASTFM_SECRET, SONA_DISCORD_CLIENT_ID
#                   compiled in. Only with -trimpath: Go records -ldflags in
#                   the binary's build info otherwise. --no-credentials leaves
#                   them out, for a build without it.
#   UPDATE_FEED, UPDATE_KEY   the self-update feed URL and public key.
# --update TARGET (darwin-arm64, windows-amd64: a release build) sets both
# from this repository's latest release and internal/update/update.pub when
# UPDATE_FEED is unset and GITHUB_REPOSITORY is; an empty update.pub warns that
# the build will not self-update (stderr, never the flags).
#
# Callers that run `go build` where a command line is logged keep the Last.fm
# secret out of the log (a Taskfile command is `silent`).
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
goos=linux update='' creds=1
while (($#)); do
  case $1 in
    --goos) goos=${2:?--goos needs a value}; shift 2 ;;
    --update) update=${2:?--update needs a target}; shift 2 ;;
    --no-credentials) creds=; shift ;;
    *) echo "ldflags.sh: unknown argument $1" >&2; exit 2 ;;
  esac
done

if [[ -n $update && -z ${UPDATE_FEED:-} && -n ${GITHUB_REPOSITORY:-} ]]; then
  if [[ -s $repo/internal/update/update.pub ]]; then
    UPDATE_FEED="https://github.com/$GITHUB_REPOSITORY/releases/latest/download/update-$update.json"
    UPDATE_KEY=$(cat "$repo/internal/update/update.pub")
  else
    echo "::warning::internal/update/update.pub is empty: this build will not self-update" >&2
  fi
fi

flags=(-w -s)
[[ $goos == windows ]] && flags+=(-H windowsgui)
flags+=(-X "main.version=${VERSION:-$(cat "$repo/VERSION")}" -X github.com/egoist/mygo.production=1)
[[ $goos == darwin && -n ${MACOSX_DEPLOYMENT_TARGET:-} ]] && flags+=("-extldflags=-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET")
[[ -n ${UPDATE_FEED:-} ]] && flags+=(-X "github.com/egoist/mygo.packageUpdateFeed=$UPDATE_FEED")
[[ -n ${UPDATE_KEY:-} ]] && flags+=(-X "github.com/egoist/mygo.packageUpdateKey=$UPDATE_KEY")
if [[ -n $creds ]]; then
  [[ -n ${SONA_LASTFM_KEY:-} ]] && flags+=(-X "github.com/coredev-uk/sona-go/internal/scrobble.lastfmKey=$SONA_LASTFM_KEY")
  [[ -n ${SONA_LASTFM_SECRET:-} ]] && flags+=(-X "github.com/coredev-uk/sona-go/internal/scrobble.lastfmSecret=$SONA_LASTFM_SECRET")
  [[ -n ${SONA_DISCORD_CLIENT_ID:-} ]] && flags+=(-X "github.com/coredev-uk/sona-go/internal/discord.clientID=$SONA_DISCORD_CLIENT_ID")
fi
echo "${flags[*]}"
