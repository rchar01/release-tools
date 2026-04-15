#!/usr/bin/env bash
set -euo pipefail

source "$(dirname -- "$0")/common.sh"

tag="$(resolve_tag)"
notes_mode="$(release_notes_mode)"
notes_source="$(release_notes_source)"

ensure_tmp_dir

case "$notes_mode" in
news-md)
	news_file="$REPO_ROOT/$notes_source"
	[[ -f "$news_file" ]] || err "release notes source not found: ${news_file}"
	news_section="$(extract_news_section "$news_file" "$tag")"
	notes_file="$TMP_DIR/release-notes-${tag}.md"
	{
		if [[ -n "$news_section" ]]; then
			printf '%s\n' "$news_section"
		else
			printf -- '- No summary entry found in `%s`.\n' "$notes_source"
		fi
	} >"$notes_file"
	printf '%s\n' "$notes_file"
	;;
none)
	err 'release notes generation is disabled for this repository'
	;;
*)
	err "unsupported RELEASE_NOTES_MODE: ${notes_mode}"
	;;
esac
