# Release Tools Usage

This document describes the `release-tools` v2 CLI-only runtime-bootstrap model.

The goal is:

- keep shared release behavior in `release-tools`
- keep project-specific release config in the consuming repo
- use `bin/release-tools` as the only release entrypoint
- pin an exact `release-tools` tag in the consuming repo config
- bootstrap that toolkit automatically at runtime

Ready-to-copy starting points are included in this repository:

- `examples/bootstrap-release-tools.sh`
- `examples/.release-tools.env`
- `examples/forgejo-release.yml`

## Required Project Files

A consuming project should have at least these files:

- `.release-tools.env`
- `scripts/bootstrap-release-tools.sh`
- `.goreleaser.yaml`
- `NEWS.md` if release notes are generated from changelog entries
- CI release workflow if publishing from CI

Depending on the project, it may also have:

- `scripts/build.sh`
- `scripts/install-<project>.sh`
- `scripts/install-release-<project>.sh`

## 1. Define Release Tools Config

Add `.release-tools.env` at the repository root.

Example:

```sh
RELEASE_PROJECT=platformctl
RELEASE_TOOLS_VERSION=v2.1.0
RELEASE_OWNER=rch
RELEASE_REPO=platformctl
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml
```

Rules:

- pin only real released tags such as `v2.1.0`
- do not pin branches or commits
- allow temporary override through `RELEASE_TOOLS_VERSION=vX.Y.Z`

## 2. Add A Bootstrap Script

Add `scripts/bootstrap-release-tools.sh`.

Responsibilities:

- read `RELEASE_TOOLS_VERSION` from the environment or `.release-tools.env`
- validate tag format `vX.Y.Z`
- download the matching `release-tools` archive into `.tmp/release-tools/<version>`
- fall back to a pinned git checkout when the archive is not available
- update `.tmp/release-tools/current`
- print the resolved checkout path

Use `examples/bootstrap-release-tools.sh` in this repository as a ready-to-copy
starting point.

Notes:

- this script should not implement release logic
- it should only fetch/select the toolkit checkout
- keep it project-agnostic except for repo-local paths
- archive bootstrap requires `curl` and `tar`; fallback source checkout requires `git`

## 3. Review Supported Variables

Supported `.release-tools.env` keys:

- `RELEASE_PROJECT`
- `RELEASE_TOOLS_VERSION`
- `RELEASE_OWNER`
- `RELEASE_REPO`
- `RELEASE_API_URL`
- `RELEASE_DOWNLOAD_URL`
- `RELEASE_NOTES_SOURCE`
- `RELEASE_NOTES_MODE`
- `RELEASE_BODY_MODE`
- `GORELEASER_CONFIG`
- `GORELEASER_BIN`
- `RELEASE_REQUIRE_GO`

Supported environment-only variables:

- `RELEASE_CONFIG_FILE`
- `CODEBERG_TOKEN`
- `VERSION`

Environment variables override `.release-tools.env` values. Set
`RELEASE_CONFIG_FILE` to load a different config file.

Required for release commands:

- `RELEASE_PROJECT`
- `RELEASE_OWNER`

Additionally required for `release-tools publish-tag`:

- `VERSION` or a positional tag argument such as `v2.1.0`

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

- `release-tools doctor`
- `release-tools check`
- `release-tools snapshot`
- `release-tools publish`
- `release-tools publish-tag vX.Y.Z`
- `release-tools notes vX.Y.Z`

Recommended for Go projects:

- use repo-local `.tmp/go-build` and `.tmp/go-cache` in hooks
- set `RELEASE_REQUIRE_GO=1` if the release flow requires a local Go toolchain
- keep archives to the exact contract expected by downstream consumers
- keep binary names stable across builds and installs

Shell or documentation toolkits can use GoReleaser meta archives or source
archives without configuring a project Go build. `release-tools` itself ships as
a compiled Go CLI, but consuming projects only need Go when their own release
flow requires it.

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

The format must match what `release-tools` expects when extracting notes for a
specific tag.

Generated release notes are release body text only. The toolkit does not add a
top-level Markdown heading for the tag because the forge already renders the
release title separately.

## 6. Add CI Release Workflow

CI should:

- checkout full history
- install GoReleaser
- run project tests
- bootstrap `release-tools`
- run `bin/release-tools publish` from the bootstrapped checkout

Use `examples/forgejo-release.yml` in this repository as a ready-to-copy
starting point.

## 7. Token And Auth Contract

The public token name is:

- `CODEBERG_TOKEN`

Local maintainer flow should support:

- `CODEBERG_TOKEN` from the environment
- `~/.config/codeberg/token`

Recommended local setup:

```bash
export CODEBERG_TOKEN="$(cat ~/.config/codeberg/token)"
```

CI should provide `CODEBERG_TOKEN` through repository secrets.

`release-tools` maps that internally to `GITEA_TOKEN` only for the GoReleaser
process. The public consumer contract stays `CODEBERG_TOKEN`.

## 8. Runtime Contract And Command Surface

`release-tools publish-tag` publishes from a clean clone of the exact repo tag,
while running the current bootstrapped toolkit CLI against that clone.

The CLI fails fast when required variables are missing:

- `release-tools doctor`
- `release-tools check`
- `release-tools snapshot`
- `release-tools publish`
- `release-tools notes`
  require `RELEASE_PROJECT` and `RELEASE_OWNER`
- `release-tools publish-tag`
  requires `RELEASE_PROJECT`, `RELEASE_OWNER`, and `VERSION` or a positional tag

After integration, these commands should work from the consumer repo root:

```bash
toolkit_dir="$(./scripts/bootstrap-release-tools.sh)"
"$toolkit_dir/bin/release-tools" doctor
"$toolkit_dir/bin/release-tools" check
"$toolkit_dir/bin/release-tools" snapshot
"$toolkit_dir/bin/release-tools" publish
"$toolkit_dir/bin/release-tools" publish-tag v2.1.0
"$toolkit_dir/bin/release-tools" notes v2.1.0
```

For an older project tag that predates `RELEASE_TOOLS_VERSION` in
`.release-tools.env`:

```bash
RELEASE_TOOLS_VERSION=v2.1.0 VERSION=v1.0.0 ./scripts/bootstrap-release-tools.sh
```

Then call `bin/release-tools publish-tag v1.0.0` from the printed toolkit path.

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
- Goreleaser invocation wrapper
- check/snapshot/publish logic
- safe `publish-tag` clean-clone logic
- release-notes generation logic
- release body patch logic

The consuming project should not duplicate these behaviors.

## 11. Minimum Verification Checklist

After wiring a project to `release-tools`, verify:

1. `toolkit_dir="$(./scripts/bootstrap-release-tools.sh)"`
2. `"$toolkit_dir/bin/release-tools" doctor`
3. `"$toolkit_dir/bin/release-tools" check`
4. `"$toolkit_dir/bin/release-tools" snapshot`
5. `"$toolkit_dir/bin/release-tools" notes <current-tag>`
6. `"$toolkit_dir/bin/release-tools" publish-tag <older-project-tag>`

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
├── scripts/
│   ├── bootstrap-release-tools.sh
│   ├── build.sh
│   └── install-release-<project>.sh
└── <ci-release-workflow>
```

That is the configuration `release-tools` assumes and documents for CLI-only
runtime-bootstrap consumers.
