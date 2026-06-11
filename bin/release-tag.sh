#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=bin/common.sh
source "$(dirname -- "$0")/common.sh"

tag="$(resolve_tag)"
token="$(resolve_token)"

if ! git -C "$REPO_ROOT" rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
	err "tag ${tag} does not exist locally"
fi

clone_dir="$TMP_DIR/release-${tag}"
toolkit_dir="$(toolkit_root)"

cleanup() {
	rm -rf "$clone_dir"
}

trap cleanup EXIT

ensure_tmp_dir
rm -rf "$clone_dir"

log "Creating temporary clone for ${tag}"
git clone --quiet --branch "$tag" --depth 1 "file://$REPO_ROOT/.git" "$clone_dir"

[[ -d "$toolkit_dir/bin" ]] || err "expected toolkit bin directory at ${toolkit_dir}/bin"

notes_file="$(RELEASE_REPO_ROOT="$clone_dir" "$toolkit_dir/bin/release-notes.sh")"

log "Publishing ${tag}"
(
	cd "$clone_dir"
	RELEASE_REPO_ROOT="$clone_dir" CODEBERG_TOKEN="$token" "$toolkit_dir/bin/run-goreleaser.sh" release --clean --release-notes "$notes_file"
)

RELEASE_REPO_ROOT="$clone_dir" VERSION="$tag" NOTES_FILE="$notes_file" CODEBERG_TOKEN="$token" "$toolkit_dir/bin/update-release-body.sh"

log "Published ${tag}"
