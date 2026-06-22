# Release Tools Usage

This document describes the `release-tools` v3 installed-CLI model.

The goal is:

- keep shared release behavior in `release-tools`
- keep project-specific release config in the consuming repo
- install one `release-tools` binary into `PATH`
- run `release-tools` directly from the project root
- avoid copied bootstrap or release helper scripts in consumer repos

Ready-to-copy starting points are included in this repository:

- `examples/.release-tools.env`
- `examples/forgejo-release.yml`

## Required Project Files

A consuming project should have at least these files:

- `.release-tools.env`
- `.goreleaser.yaml`
- `NEWS.md` if release notes are generated from changelog entries
- CI release workflow if publishing from CI

Depending on the project, it may also have project-owned build or installer
scripts, but `release-tools` does not require any copied helper script.

## 1. Install The CLI

Download the matching release binary and place it in a directory on `PATH`.

Linux amd64 example:

```bash
mkdir -p "$HOME/.local/bin"
curl -fsSL -o "$HOME/.local/bin/release-tools" \
  "https://codeberg.org/rch/release-tools/releases/download/v3.1.0/release-tools_3.1.0_linux_amd64"
chmod +x "$HOME/.local/bin/release-tools"
```

Published binary names follow this shape:

```text
release-tools_<version>_<os>_<arch>
```

Supported release binaries:

- `release-tools_3.1.0_linux_amd64`
- `release-tools_3.1.0_linux_arm64`
- `release-tools_3.1.0_darwin_amd64`
- `release-tools_3.1.0_darwin_arm64`

For system-wide installation, use a privileged install directory instead:

```bash
sudo install -m 0755 release-tools_3.1.0_linux_amd64 /usr/local/bin/release-tools
```

## 2. Define Release Tools Config

Add `.release-tools.env` at the repository root.

Example:

```sh
RELEASE_PROJECT=platformctl
RELEASE_FORGE=codeberg
RELEASE_OWNER=rch
RELEASE_REPO=platformctl
# RELEASE_TOKEN_FILE=~/.config/forge/token
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml
```

Supported `RELEASE_FORGE` values:

- `codeberg`
- `gitea`
- `forgejo`
- `github`
- `gitlab`

`codeberg`, `gitea`, and `forgejo` use Codeberg-compatible defaults unless
`RELEASE_API_URL` is set explicitly.

## 3. Review Supported Variables

Supported `.release-tools.env` keys:

- `RELEASE_PROJECT`
- `RELEASE_FORGE`
- `RELEASE_OWNER`
- `RELEASE_REPO`
- `RELEASE_API_URL`
- `RELEASE_NOTES_SOURCE`
- `RELEASE_NOTES_MODE`
- `RELEASE_BODY_MODE`
- `GORELEASER_CONFIG`
- `GORELEASER_BIN`
- `RELEASE_REQUIRE_GO`
- `RELEASE_TOKEN_FILE`

Supported environment-only variables:

- `RELEASE_CONFIG_FILE`
- `RELEASE_TOKEN`
- native GoReleaser token variables: `GITEA_TOKEN`, `GITHUB_TOKEN`, or
  `GITLAB_TOKEN`
- `VERSION`

Environment variables override `.release-tools.env` values. Set
`RELEASE_CONFIG_FILE` to load a different config file.

Required for release commands:

- `RELEASE_PROJECT`
- `RELEASE_OWNER`

Additionally required for `release-tools publish-tag`:

- `VERSION` or a positional tag argument such as `v3.1.0`

## 4. Add GoReleaser Configuration

The consuming repo owns `.goreleaser.yaml`.

`release-tools` does not define:

- build matrix
- archive names
- binary names
- checksum names
- before hooks

The project should define those directly.

The config should be compatible with these CLI commands:

- `release-tools version`
- `release-tools doctor`
- `release-tools check`
- `release-tools snapshot`
- `release-tools publish`
- `release-tools publish-tag vX.Y.Z`
- `release-tools notes vX.Y.Z`
- `release-tools completion bash|zsh|fish|powershell`

Recommended for Go projects:

- use repo-local `.tmp/go-build` and `.tmp/go-cache` in hooks
- set `RELEASE_REQUIRE_GO=1` if the release flow requires a local Go toolchain
- keep archives to the exact contract expected by downstream consumers
- keep binary names stable across builds and installs

Shell or documentation toolkits can use GoReleaser meta archives or source
archives without configuring a project Go build. `release-tools` itself ships as
a compiled Go CLI. Consuming projects only need Go when their own release flow
requires it.

## 5. Add A Release Notes Source

If the project uses `RELEASE_NOTES_MODE=news-md`, add `NEWS.md` with sections
matching release tags.

Example shape:

```md
## v2.0.0 - 2026-06-11

- switch to the CLI-only release workflow
- add self-release support

## v1.2.1 - 2026-06-01

- clarify the release-tools value proposition
```

Generated release notes are release body text only. The toolkit does not add a
top-level Markdown heading for the tag because the forge already renders the
release title separately.

## 6. Add CI Release Workflow

CI should:

- checkout full history
- install GoReleaser
- install a pinned `release-tools` binary
- run project tests
- run `release-tools publish` from the project root

Use `examples/forgejo-release.yml` in this repository as a ready-to-copy
starting point.

## 7. Token And Auth Contract

The public token name is:

- `RELEASE_TOKEN`

Local maintainer flow should support:

- `RELEASE_TOKEN` from the environment
- or the native GoReleaser token variable for `RELEASE_FORGE`
- or `RELEASE_TOKEN_FILE` pointing at a local token file

Recommended local setup:

```sh
RELEASE_TOKEN_FILE=~/.config/forge/token
```

CI should provide `RELEASE_TOKEN` through repository secrets.

Token resolution order is:

1. `RELEASE_TOKEN`
2. native forge token variable, such as `GITEA_TOKEN`
3. `RELEASE_TOKEN_FILE`

`RELEASE_TOKEN_FILE` supports `~`, `$HOME`, and `${HOME}` at the start of the
path. The file content is read only for `publish` and `publish-tag`; trailing
newlines are trimmed and the token is not printed.

`release-tools` maps that internally to the token variable GoReleaser expects:

- `RELEASE_FORGE=codeberg`, `gitea`, or `forgejo`: `GITEA_TOKEN`
- `RELEASE_FORGE=github`: `GITHUB_TOKEN`
- `RELEASE_FORGE=gitlab`: `GITLAB_TOKEN`

## 8. Runtime Contract And Command Surface

Run commands from the consumer repo root:

```bash
release-tools version
release-tools doctor
release-tools check
release-tools snapshot
release-tools publish
release-tools publish-tag v3.1.0
release-tools notes v3.1.0
```

The CLI also generates shell completion scripts:

```bash
release-tools completion bash
release-tools completion zsh
release-tools completion fish
release-tools completion powershell
```

Install or source the generated script according to your shell's completion
setup. Completion generation, `help`, and `version` do not require a release
config file.

`release-tools publish-tag` publishes from a clean full-history clone detached at
the exact repo tag. Keeping tag history available lets GoReleaser discover the
previous tag for changelog generation.

The CLI fails fast when required variables are missing:

- `release-tools doctor`
- `release-tools check`
- `release-tools snapshot`
- `release-tools publish`
- `release-tools notes`
  require `RELEASE_PROJECT` and `RELEASE_OWNER`
- `release-tools publish-tag`
  requires `RELEASE_PROJECT`, `RELEASE_OWNER`, and `VERSION` or a positional tag

## 9. What Should Stay In The Consumer Repo

These stay local to the project:

- `.goreleaser.yaml`
- `.release-tools.env`
- `NEWS.md`
- project build/install scripts
- CI workflow details
- artifact naming contract

## 10. What Should Stay In release-tools

These stay shared in `release-tools`:

- CLI command dispatch
- tool checks
- config loading
- token resolution helpers
- GoReleaser invocation wrapper
- check/snapshot/publish logic
- safe `publish-tag` clean-clone logic
- release-notes generation logic
- release body patch logic

The consuming project should not duplicate these behaviors.

## 11. Minimum Verification Checklist

After wiring a project to `release-tools`, verify:

1. `release-tools version`
2. `release-tools doctor`
3. `release-tools check`
4. `release-tools snapshot`
5. `release-tools notes <current-tag>`
6. `release-tools publish-tag <older-project-tag>`

For the last command, it is fine to test with an intentionally invalid token if
you only want to verify the flow gets through clone, notes, and build without
actually publishing.

Older project tags can still fail if the tagged source tree does not contain the
project files expected by the current toolkit, such as `.goreleaser.yaml` or
`NEWS.md` when `RELEASE_NOTES_MODE=news-md` is used.

## 12. Recommended Consumer Template

For a new project, the minimum integration shape is:

```text
.
├── .goreleaser.yaml
├── .release-tools.env
├── NEWS.md
└── <ci-release-workflow>
```

That is the configuration `release-tools` assumes and documents for installed
CLI consumers.
