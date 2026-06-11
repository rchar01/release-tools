#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=bin/common.sh
source "$(dirname -- "$0")/common.sh"

cd "$REPO_ROOT"
"$TOOLKIT_DIR/bin/run-goreleaser.sh" release --snapshot --skip=publish --clean
