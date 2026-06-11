#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=bin/common.sh
source "$(dirname -- "$0")/common.sh"

"$TOOLKIT_DIR/bin/run-goreleaser.sh" check
