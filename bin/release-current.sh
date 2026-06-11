#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=bin/common.sh
source "$(dirname -- "$0")/common.sh"

tag="$(resolve_tag)"
token="$(resolve_token)"
notes_file="$("$TOOLKIT_DIR/bin/release-notes.sh")"

cd "$REPO_ROOT"
CODEBERG_TOKEN="$token" "$TOOLKIT_DIR/bin/run-goreleaser.sh" release --clean --release-notes "$notes_file"
VERSION="$tag" NOTES_FILE="$notes_file" CODEBERG_TOKEN="$token" "$TOOLKIT_DIR/bin/update-release-body.sh"
