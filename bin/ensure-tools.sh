#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=bin/common.sh
source "$(dirname -- "$0")/common.sh"

if [[ "${RELEASE_REQUIRE_GO:-0}" == "1" ]]; then
	require_cmd go
fi

resolve_goreleaser_bin >/dev/null
