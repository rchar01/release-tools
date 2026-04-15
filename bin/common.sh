#!/usr/bin/env bash
set -euo pipefail

TOOLKIT_DIR="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="${RELEASE_REPO_ROOT:-$(pwd)}"
TMP_DIR="${RELEASE_TMP_DIR:-${REPO_ROOT}/.tmp}"

log() {
	printf '[INFO] %s\n' "$1"
}

err() {
	printf '[ERROR] %s\n' "$1" >&2
	exit 1
}

require_cmd() {
	command -v "$1" >/dev/null 2>&1 || err "$1 is required"
}

repo_root() {
	printf '%s\n' "$REPO_ROOT"
}

toolkit_root() {
	printf '%s\n' "$TOOLKIT_DIR"
}

ensure_tmp_dir() {
	mkdir -p "$TMP_DIR"
}

release_project() {
	printf '%s\n' "${RELEASE_PROJECT:-}"
}

release_owner() {
	[[ -n "${RELEASE_OWNER:-}" ]] || err 'RELEASE_OWNER is required'
	printf '%s\n' "$RELEASE_OWNER"
}

release_repo() {
	local repo
	repo="${RELEASE_REPO:-${RELEASE_PROJECT:-}}"
	[[ -n "$repo" ]] || err 'RELEASE_REPO or RELEASE_PROJECT is required'
	printf '%s\n' "$repo"
}

release_api_url() {
	printf '%s\n' "${RELEASE_API_URL:-https://codeberg.org/api/v1}"
}

release_download_url() {
	printf '%s\n' "${RELEASE_DOWNLOAD_URL:-https://codeberg.org}"
}

release_notes_source() {
	printf '%s\n' "${RELEASE_NOTES_SOURCE:-NEWS.md}"
}

release_notes_mode() {
	printf '%s\n' "${RELEASE_NOTES_MODE:-news-md}"
}

release_body_mode() {
	printf '%s\n' "${RELEASE_BODY_MODE:-none}"
}

goreleaser_config() {
	printf '%s\n' "${GORELEASER_CONFIG:-.goreleaser.yaml}"
}

resolve_token() {
	if [[ -n "${CODEBERG_TOKEN:-}" ]]; then
		printf '%s\n' "$CODEBERG_TOKEN"
		return
	fi

	if [[ -r "${HOME}/.config/codeberg/token" ]]; then
		tr -d '\r\n' <"${HOME}/.config/codeberg/token"
		return
	fi

	err 'CODEBERG_TOKEN is required. Export it from ~/.config/codeberg/token or set it directly in the environment.'
}

resolve_goreleaser_bin() {
	if [[ -n "${GORELEASER_BIN:-}" ]]; then
		printf '%s\n' "$GORELEASER_BIN"
		return
	fi

	if command -v goreleaser >/dev/null 2>&1; then
		command -v goreleaser
		return
	fi

	if [[ -x "${HOME}/.local/bin/goreleaser" ]]; then
		printf '%s\n' "${HOME}/.local/bin/goreleaser"
		return
	fi

	if [[ -x "${HOME}/go/bin/goreleaser" ]]; then
		printf '%s\n' "${HOME}/go/bin/goreleaser"
		return
	fi

	if [[ -x "${HOME}/.local/go/bin/goreleaser" ]]; then
		printf '%s\n' "${HOME}/.local/go/bin/goreleaser"
		return
	fi

	if [[ -x "/usr/local/bin/goreleaser" ]]; then
		printf '%s\n' '/usr/local/bin/goreleaser'
		return
	fi

	if [[ -x "/usr/bin/goreleaser" ]]; then
		printf '%s\n' '/usr/bin/goreleaser'
		return
	fi

	err 'goreleaser not found. Install it and ensure it is available in PATH or GORELEASER_BIN.'
}

resolve_tag() {
	local tag
	tag="${VERSION:-${TAG:-}}"
	if [[ -n "$tag" ]]; then
		printf '%s\n' "$tag"
		return
	fi

	tag="$(git -C "$REPO_ROOT" describe --tags --exact-match 2>/dev/null || true)"
	if [[ -n "$tag" ]]; then
		printf '%s\n' "$tag"
		return
	fi

	err 'VERSION or TAG is required when the current commit is not an exact tag'
}

extract_news_section() {
	local file="$1"
	local tag="$2"
	awk -v start_pattern="^## ${tag} - " '
		$0 ~ start_pattern { in_section = 1; next }
		in_section && $0 ~ /^## / { exit }
		in_section { print }
	' "$file" | awk '
		{ lines[NR] = $0 }
		END {
			start = 1
			while (start <= NR && lines[start] ~ /^[[:space:]]*$/) start++
			end = NR
			while (end >= start && lines[end] ~ /^[[:space:]]*$/) end--
			for (i = start; i <= end; i++) print lines[i]
		}
	'
}
