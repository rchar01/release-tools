#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=bin/common.sh
source "$(dirname -- "$0")/common.sh"

tag="$(resolve_tag)"
notes_file="${NOTES_FILE:-}"
body_mode="$(release_body_mode)"

if [[ "$body_mode" == "none" ]]; then
	exit 0
fi

[[ "$body_mode" == 'patch' ]] || err "unsupported RELEASE_BODY_MODE: ${body_mode}"

token="$(resolve_token)"

if [[ -z "$notes_file" ]]; then
	notes_file="$("$TOOLKIT_DIR/bin/release-notes.sh")"
fi

[[ -f "$notes_file" ]] || err "release notes file not found: ${notes_file}"

require_cmd curl

json_get() {
	if command -v jq >/dev/null 2>&1; then
		jq -r "$1"
		return
	fi
	python3 -c 'import json,sys; data=json.load(sys.stdin); path=sys.argv[1].split("."); cur=data
for part in path:
    if not part:
        continue
    cur=cur[part]
print(cur)' "$1"
}

json_payload() {
	local body_file="$1"
	if command -v jq >/dev/null 2>&1; then
		jq -n --rawfile body "$body_file" '{body:$body}'
		return
	fi
	python3 -c 'import json, pathlib, sys; print(json.dumps({"body": pathlib.Path(sys.argv[1]).read_text()}))' "$body_file"
}

release_json="$({ curl -fsSL -H "Authorization: token ${token}" "$(release_api_url)/repos/$(release_owner)/$(release_repo)/releases/tags/${tag}"; })"
release_id="$(printf '%s' "$release_json" | json_get '.id')"
payload="$(json_payload "$notes_file")"

curl -fsSL -X PATCH \
	-H "Authorization: token ${token}" \
	-H 'Content-Type: application/json' \
	-d "$payload" \
	"$(release_api_url)/repos/$(release_owner)/$(release_repo)/releases/${release_id}" >/dev/null

log "Updated release body for ${tag}"
