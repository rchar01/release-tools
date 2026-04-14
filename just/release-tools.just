release_project := env_var_or_default("RELEASE_PROJECT", "")
release_owner := env_var_or_default("RELEASE_OWNER", "")
release_repo := env_var_or_default("RELEASE_REPO", release_project)
release_api_url := env_var_or_default("RELEASE_API_URL", "https://codeberg.org/api/v1")
release_download_url := env_var_or_default("RELEASE_DOWNLOAD_URL", "https://codeberg.org")
release_notes_source := env_var_or_default("RELEASE_NOTES_SOURCE", "NEWS.md")
release_notes_mode := env_var_or_default("RELEASE_NOTES_MODE", "news-md")
release_body_mode := env_var_or_default("RELEASE_BODY_MODE", "none")
goreleaser_config := env_var_or_default("GORELEASER_CONFIG", ".goreleaser.yaml")

release_env := 'RELEASE_PROJECT="' + release_project + '" RELEASE_OWNER="' + release_owner + '" RELEASE_REPO="' + release_repo + '" RELEASE_API_URL="' + release_api_url + '" RELEASE_DOWNLOAD_URL="' + release_download_url + '" RELEASE_NOTES_SOURCE="' + release_notes_source + '" RELEASE_NOTES_MODE="' + release_notes_mode + '" RELEASE_BODY_MODE="' + release_body_mode + '" GORELEASER_CONFIG="' + goreleaser_config + '"'

release-check:
    {{release_env}} ./tools/release/bin/release-check.sh

release-snapshot:
    {{release_env}} ./tools/release/bin/release-snapshot.sh

release:
    {{release_env}} ./tools/release/bin/release-current.sh

release-tag:
    {{release_env}} ./tools/release/bin/release-tag.sh

release-notes:
    {{release_env}} ./tools/release/bin/release-notes.sh
