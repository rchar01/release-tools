#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "$0")/common.sh"

"$TOOLKIT_DIR/bin/run-goreleaser.sh" build --snapshot --clean
