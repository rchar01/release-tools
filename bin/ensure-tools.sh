#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "$0")/common.sh"

require_cmd go
resolve_goreleaser_bin >/dev/null
