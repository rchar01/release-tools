#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
TMP_DIR="$ROOT_DIR/.tmp/release-tools"
REPO_URL="${RELEASE_TOOLS_REPO_URL:-https://codeberg.org/rch/release-tools}"
VERSION_FILE="$ROOT_DIR/.release-tools-version"

version="${RELEASE_TOOLS_VERSION:-}"
if [[ -z "$version" ]]; then
	if [[ ! -f "$VERSION_FILE" ]]; then
		printf '%s\n' 'missing .release-tools-version; set RELEASE_TOOLS_VERSION=vX.Y.Z or add the file to the repository root' >&2
		exit 1
	fi
	version="$(tr -d '\r\n' <"$VERSION_FILE")"
fi

case "$version" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
	printf '%s\n' "invalid release-tools version '$version'; expected a tag like v1.3.0" >&2
	exit 1
	;;
esac

checkout_dir="$TMP_DIR/$version"
current_link="$TMP_DIR/current"

mkdir -p "$TMP_DIR"
if [[ ! -d "$checkout_dir/.git" ]]; then
	rm -rf "$checkout_dir"
	git clone --quiet --branch "$version" --depth 1 "$REPO_URL" "$checkout_dir"
fi

ln -sfn "$checkout_dir" "$current_link"

printf '%s\n' "$checkout_dir"
