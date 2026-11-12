#!/usr/bin/env bash
# no-deploy-warnings.sh LOG: fails when the macOS linker warned, in LOG, that an
# object was built for a newer macOS than the link targets (every cgo object and
# the link must use build/darwin/MIN_MACOS, which setup-macos exports).
set -euo pipefail
if grep 'built for newer macOS version' "${1:?log}"; then
  echo "::error::objects or libraries target a newer macOS than the link (build/darwin/MIN_MACOS)"
  exit 1
fi
