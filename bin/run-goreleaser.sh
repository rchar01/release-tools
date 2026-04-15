#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "$0")/common.sh"

goreleaser_bin="$(resolve_goreleaser_bin)"
token="$(resolve_token)"
cd "$REPO_ROOT"
exec env GITEA_TOKEN="$token" "$goreleaser_bin" --config "$(goreleaser_config)" "$@"
