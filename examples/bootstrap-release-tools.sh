#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
TMP_DIR="$ROOT_DIR/.tmp/release-tools"
REPO_URL="${RELEASE_TOOLS_REPO_URL:-https://codeberg.org/rch/release-tools}"
DOWNLOAD_URL="${RELEASE_TOOLS_DOWNLOAD_URL:-https://codeberg.org}"
DOWNLOAD_OWNER="${RELEASE_TOOLS_OWNER:-rch}"
DOWNLOAD_REPO="${RELEASE_TOOLS_REPO:-release-tools}"
CONFIG_FILE="${RELEASE_CONFIG_FILE:-$ROOT_DIR/.release-tools.env}"

read_config_value() {
	local key="$1"
	local line line_key value
	[[ -f "$CONFIG_FILE" ]] || return 1

	while IFS= read -r line || [[ -n "$line" ]]; do
		line="${line%$'\r'}"
		[[ -n "$line" ]] || continue
		[[ "$line" == \#* ]] && continue
		[[ "$line" == *=* ]] || continue

		line_key="${line%%=*}"
		[[ "$line_key" == "$key" ]] || continue

		value="${line#*=}"
		value="${value%\"}"
		value="${value#\"}"
		value="${value%\'}"
		value="${value#\'}"
		printf '%s\n' "$value"
		return 0
	done <"$CONFIG_FILE"

	return 1
}

version="${RELEASE_TOOLS_VERSION:-}"
if [[ -z "$version" ]]; then
	if ! version="$(read_config_value RELEASE_TOOLS_VERSION)"; then
		printf '%s\n' 'missing RELEASE_TOOLS_VERSION; set it in the environment or .release-tools.env' >&2
		exit 1
	fi
fi

case "$version" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
	printf '%s\n' "invalid release-tools version '$version'; expected a tag like v1.3.0" >&2
	exit 1
	;;
esac

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*)
	printf '%s\n' "unsupported OS for release-tools archive: $(uname -s)" >&2
	exit 1
	;;
esac

case "$(uname -m)" in
x86_64|amd64) arch=amd64 ;;
arm64|aarch64) arch=arm64 ;;
*)
	printf '%s\n' "unsupported architecture for release-tools archive: $(uname -m)" >&2
	exit 1
	;;
esac

checkout_dir="$TMP_DIR/$version/$os-$arch"
current_link="$TMP_DIR/current"
version_number="${version#v}"
archive_url="${DOWNLOAD_URL%/}/${DOWNLOAD_OWNER}/${DOWNLOAD_REPO}/releases/download/${version}/release-tools_${version_number}_${os}_${arch}.tar.gz"

mkdir -p "$TMP_DIR"
if [[ ! -x "$checkout_dir/bin/release-tools" ]]; then
	rm -rf "$checkout_dir"
	mkdir -p "$checkout_dir"
	if command -v curl >/dev/null 2>&1 && command -v tar >/dev/null 2>&1; then
		if curl -fsSL "$archive_url" | tar -xz -C "$checkout_dir" --strip-components=1; then
			:
		else
			rm -rf "$checkout_dir"
		fi
	fi

	if [[ ! -x "$checkout_dir/bin/release-tools" ]]; then
		rm -rf "$checkout_dir"
		git clone --quiet --branch "$version" --depth 1 "$REPO_URL" "$checkout_dir"
	fi
fi

ln -sfn "$checkout_dir" "$current_link"

printf '%s\n' "$checkout_dir"
