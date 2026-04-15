# Release Tools Usage

This document describes what a Go project should contain to use
`release-tools` with the Make-only runtime-bootstrap model.

The goal is:

- keep shared release behavior in `release-tools`
- keep project-specific configuration in the consuming repo
- use `make` as the only release entrypoint
- pin an exact `release-tools` tag in the consuming repo
- bootstrap that toolkit automatically at runtime

Ready-to-copy starting points are included in this repository:

- `examples/bootstrap-release-tools.sh`
- `examples/Makefile.release-tools`
- `examples/.release-tools-version`
- `examples/forgejo-release.yml`

## Required Project Files

A consuming project should have at least these files:

- `.release-tools-version`
- `Makefile`
- `scripts/bootstrap-release-tools.sh`
- `.goreleaser.yaml`
- `NEWS.md` if release notes are generated from changelog entries
- `.forgejo/workflows/release.yml` or equivalent CI release workflow

Depending on the project, it may also have:

- `scripts/build.sh`
- `scripts/install-<project>.sh`
- `scripts/install-release-<project>.sh`

## 1. Pin The Toolkit Version

Add `.release-tools-version` at the repository root.

Example:

```text
v1.1.0
```

Rules:

- pin only real released tags such as `v1.1.0`
- do not pin branches or commits
- allow temporary override through `RELEASE_TOOLS_VERSION=vX.Y.Z`

## 2. Add A Bootstrap Script

Add `scripts/bootstrap-release-tools.sh`.

Responsibilities:

- read `.release-tools-version` unless `RELEASE_TOOLS_VERSION` is set
- validate tag format `vX.Y.Z`
- clone `release-tools` into `.tmp/release-tools/<version>`
- update `.tmp/release-tools/current`
- print the resolved checkout path

Use `examples/bootstrap-release-tools.sh` in this repository as a ready-to-copy
starting point.

Notes:

- this script should not implement release logic
- it should only fetch/select the toolkit checkout
- keep it project-agnostic except for repo-local paths

## 3. Configure The Main Makefile

The main `Makefile` should:

- resolve the pinned toolkit version
- run the bootstrap script during parse
- set the project-specific release variables
- include the shared `release-tools` make module

Use `examples/Makefile.release-tools` in this repository as a ready-to-copy
starting point.

Important details:

- pass `RELEASE_TOOLS_VERSION` into the bootstrap script explicitly
- otherwise `make RELEASE_TOOLS_VERSION=vX.Y.Z ...` may not affect bootstrap
- keep project-specific values in the consumer `Makefile`, not in `release-tools`
- `RELEASE_REPO` is optional if it matches `RELEASE_PROJECT`

## 4. Define Project-Specific Release Variables

These values belong in the consuming repo `Makefile`.

Typical variables:

- `RELEASE_PROJECT`: project name
- `RELEASE_OWNER`: forge owner/org
- `RELEASE_REPO`: forge repo name
- `RELEASE_NOTES_SOURCE`: release notes source file, usually `NEWS.md`
- `RELEASE_NOTES_MODE`: usually `news-md`
- `RELEASE_BODY_MODE`: usually `patch`
- `GORELEASER_CONFIG`: usually `.goreleaser.yaml`

Example:

```make
RELEASE_PROJECT := platformctl
RELEASE_OWNER := rch
RELEASE_REPO := platformctl
RELEASE_NOTES_SOURCE := NEWS.md
RELEASE_NOTES_MODE := news-md
RELEASE_BODY_MODE := patch
GORELEASER_CONFIG := .goreleaser.yaml
```

Expected consumer variables supported by `release-tools`:

- `RELEASE_PROJECT`
- `RELEASE_OWNER`
- `RELEASE_REPO`
- `RELEASE_API_URL`
- `RELEASE_DOWNLOAD_URL`
- `RELEASE_NOTES_SOURCE`
- `RELEASE_NOTES_MODE`
- `RELEASE_BODY_MODE`
- `GORELEASER_CONFIG`
- `VERSION`

Required for release commands:

- `RELEASE_PROJECT`
- `RELEASE_OWNER`

Additionally required for `make release-tag`:

- `VERSION`

## 5. Add Goreleaser Configuration

The consuming repo still owns `.goreleaser.yaml`.

`release-tools` does not define:

- build matrix
- archive names
- binary names
- checksum names
- before hooks

The project should define those directly.

The config should be compatible with these shared make targets:

- `make release-check`
- `make release-snapshot`
- `make release`
- `VERSION=vX.Y.Z make release-tag`
- `VERSION=vX.Y.Z make release-notes`

Recommended:

- use repo-local `.tmp/go-build` and `.tmp/go-cache` in hooks
- keep archives to the exact contract expected by downstream consumers
- keep the binary name stable across builds and installs

## 6. Add A Release Notes Source

If the project uses `RELEASE_NOTES_MODE := news-md`, add `NEWS.md` with sections
matching release tags.

Example shape:

```md
## v1.2.0 - 2026-04-15

- improve release bootstrap
- fix old-tag publish flow

## v1.1.0 - 2026-04-14

- initial shared release-tools integration
```

The format must match what `release-tools` expects when extracting notes for a
specific tag.

Generated release notes are release body text only. The toolkit does not add a
top-level Markdown heading for the tag because the forge already renders the
release title separately.

## 7. Add CI Release Workflow

CI should:

- checkout full history
- install Go
- install Goreleaser
- run `make test`
- run `make release`

Use `examples/forgejo-release.yml` in this repository as a ready-to-copy
starting point.

## 8. Token And Auth Contract

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

`release-tools` maps that internally to `GITEA_TOKEN` for Goreleaser, but the
consumer contract stays `CODEBERG_TOKEN`.

## 9. Runtime Contract And Command Surface

`release-tag` still publishes from a clean clone of the exact repo tag, but it
uses the current bootstrapped toolkit scripts against that clone by setting
`RELEASE_REPO_ROOT`.

The shared Make frontend fails fast when required variables are missing:

- `make release-check`
- `make release-snapshot`
- `make release`
- `make release-notes`
  require `RELEASE_PROJECT` and `RELEASE_OWNER`
- `make release-tag`
  requires `RELEASE_PROJECT`, `RELEASE_OWNER`, and `VERSION`

After integration, these commands should work from the consumer repo root:

```bash
make help
make test
make release-check
make release-snapshot
make release
VERSION=v1.2.0 make release-tag
VERSION=v1.2.0 make release-notes
```

For an older project tag that predates `.release-tools-version`:

```bash
RELEASE_TOOLS_VERSION=v1.1.0 VERSION=v1.0.0 make release-tag
```

`make help` depends on the consumer repository defining its own help target.

## 10. What Should Stay In The Consumer Repo

These stay local to the project:

- `.goreleaser.yaml`
- `NEWS.md`
- project build/install scripts
- project-specific `Makefile` variables
- CI workflow details
- artifact naming contract

## 11. What Should Stay In release-tools

These stay shared in `release-tools`:

- tool checks
- token resolution helpers
- goreleaser invocation wrapper
- release-check logic
- release-snapshot logic
- release-current logic
- release-tag logic
- release-notes generation logic
- release body patch logic
- shared Make targets

The consuming project should not duplicate these behaviors.

## 12. Minimum Verification Checklist

After wiring a project to `release-tools`, verify:

1. `rm -rf .tmp/release-tools && make help`
2. `make release-check`
3. `make release-snapshot`
4. `VERSION=<current-tag> make release-notes`
5. `RELEASE_TOOLS_VERSION=<pinned-tag> VERSION=<older-project-tag> make release-tag`

For the last command, it is fine to test with an intentionally invalid token if
you only want to verify the flow gets through clone, notes, and build without
actually publishing.

Older project tags can still fail if the tagged source tree does not contain the
project files expected by the current toolkit, such as `.goreleaser.yaml` or
`NEWS.md` when `RELEASE_NOTES_MODE=news-md` is used.

## 13. Recommended Consumer Template

For a new project, the minimum integration shape is:

```text
.
├── .goreleaser.yaml
├── .release-tools-version
├── Makefile
├── NEWS.md
├── scripts/
│   ├── bootstrap-release-tools.sh
│   ├── build.sh
│   └── install-release-<project>.sh
└── .forgejo/
    └── workflows/
        └── release.yml
```

That is the configuration `release-tools` assumes and documents for Make-only
runtime-bootstrap consumers.
