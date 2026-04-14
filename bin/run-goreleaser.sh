#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "$0")/common.sh"

goreleaser_bin="$(resolve_goreleaser_bin)"
cd "$REPO_ROOT"
exec "$goreleaser_bin" --config "$(goreleaser_config)" "$@"
