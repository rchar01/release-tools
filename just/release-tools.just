set shell := ["bash", "-eu", "-c"]

# Public configuration variables for consuming repos:
#   release_project       project name used by release scripts
#   release_owner         codeberg/gitea owner
#   release_repo          repository name (defaults to release_project)
#   release_notes_source  path to release notes file
#   goreleaser_config     path to goreleaser config
#   version               tag/version for release-tag recipe
#
# Advanced/internal overrides:
#   release_api_url
#   release_download_url
#   release_notes_mode
#   release_body_mode

release_tools_dir := env_var_or_default("RELEASE_TOOLS_DIR", "tools/release")
release_project := env_var_or_default("RELEASE_PROJECT", "")
release_owner := env_var_or_default("RELEASE_OWNER", "")
release_repo := env_var_or_default("RELEASE_REPO", release_project)
release_api_url := env_var_or_default("RELEASE_API_URL", "https://codeberg.org/api/v1")
release_download_url := env_var_or_default("RELEASE_DOWNLOAD_URL", "https://codeberg.org")
release_notes_source := env_var_or_default("RELEASE_NOTES_SOURCE", "NEWS.md")
release_notes_mode := env_var_or_default("RELEASE_NOTES_MODE", "news-md")
release_body_mode := env_var_or_default("RELEASE_BODY_MODE", "none")
goreleaser_config := env_var_or_default("GORELEASER_CONFIG", ".goreleaser.yaml")
version := env_var_or_default("VERSION", "")

release_tools_bin := release_tools_dir + "/bin"

check-tools:
    {{release_tools_bin}}/ensure-tools.sh

_run script: check-tools
    RELEASE_PROJECT={{release_project}} \
    RELEASE_OWNER={{release_owner}} \
    RELEASE_REPO={{release_repo}} \
    RELEASE_API_URL={{release_api_url}} \
    RELEASE_DOWNLOAD_URL={{release_download_url}} \
    RELEASE_NOTES_SOURCE={{release_notes_source}} \
    RELEASE_NOTES_MODE={{release_notes_mode}} \
    RELEASE_BODY_MODE={{release_body_mode}} \
    GORELEASER_CONFIG={{goreleaser_config}} \
    VERSION={{version}} \
    {{script}}

release-check:
    just _run {{release_tools_bin}}/release-check.sh

release-snapshot:
    just _run {{release_tools_bin}}/release-snapshot.sh

release:
    just _run {{release_tools_bin}}/release-current.sh

release-tag:
    just _run {{release_tools_bin}}/release-tag.sh

release-notes:
    just _run {{release_tools_bin}}/release-notes.sh
