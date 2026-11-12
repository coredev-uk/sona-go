#!/usr/bin/env bash
# report-failure.sh TITLE WHAT: a scheduled run failed. Comments on the open
# issue titled TITLE, or opens it, with "WHAT: <this run's URL>". Needs
# GH_TOKEN (issues: write).
set -euo pipefail
title=${1:?title} what=${2:?what}
body="$what: $GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"
n=$(gh issue list --repo "$GITHUB_REPOSITORY" --state open --search "\"$title\" in:title" --json number --jq '.[0].number')
if [ -n "$n" ]; then
  gh issue comment "$n" --repo "$GITHUB_REPOSITORY" --body "$body"
else
  gh issue create --repo "$GITHUB_REPOSITORY" --title "$title" --body "$body"
fi
