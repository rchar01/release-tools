#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=bin/common.sh
source "$(dirname -- "$0")/common.sh"

goreleaser_bin="$(resolve_goreleaser_bin)"
cd "$REPO_ROOT"

if token="$(resolve_optional_token)"; then
	exec env GITEA_TOKEN="$token" "$goreleaser_bin" --config "$(goreleaser_config)" "$@"
fi

exec "$goreleaser_bin" --config "$(goreleaser_config)" "$@"
