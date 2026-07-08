# Agent Guide: CLI Release Flow Pattern

This document is written for agents and maintainers who want to reuse the
release toolkit pattern from `release-tools` in another project.

## Goal

Build a release flow that is:

- reproducible
- safe to run locally
- safe to run in CI
- compatible with Gitea/Forgejo, GitHub, and GitLab releases
- easy for consumer repositories to install from published artifacts
- driven by one CLI entrypoint

## Pattern Summary

The pattern used by this toolkit has seven parts:

1. GoReleaser builds or packages release artifacts and checksums
2. the installed Go `release-tools` CLI provides the stable command surface
3. `.release-tools.env` stores project-specific release configuration
4. a shared tagged `release-tools` binary owns release behavior
5. a tagged release can be published from a clean temporary clone
6. consumers install from release assets, not from source builds
7. release page text can be generated from a short repo-local notes source such as `NEWS.md`

## Core Files To Recreate

- `.release-tools.env`
- `.goreleaser.yaml`
- installed `release-tools` binary in `PATH`
- CI release workflow if publishing from CI
- repo-local install script for release assets if consumers need binary installs

This repository includes copyable consumer examples in `examples/` and a full
consumer setup guide in `docs/usage.md`.

## Why This Pattern Works

### 1. Stable Artifact Contract

GoReleaser defines the exact supported artifact names and checksums.

Benefits:

- consumers know exactly what to download
- checksums are generated consistently
- local snapshot builds resemble real releases

For Go CLIs this usually means OS/arch-specific binaries or archives. For shell
or documentation toolkits this can be a meta/source archive with no project Go
build. `release-tools` itself ships as OS/arch-specific binaries.

### 2. Safe Local Publishing

Publishing from a maintainer worktree is risky when:

- `main` is ahead of the tag
- the worktree is dirty
- helper scripts have changed after the tagged commit

`release-tools publish-tag` avoids that by cloning the repo git database and
detaching at the requested tag. The clone keeps full tag history so GoReleaser
can discover the previous release tag.

Benefits:

- the published release always matches the tag
- local uncommitted changes cannot leak into the release

### 3. Shared GoReleaser Entrypoint

The installed `release-tools` binary provides the stable command surface. Its Go
implementation does three useful things when invoking GoReleaser:

- uses Cobra and Fang for help, version flags, and generated completions
- resolves the GoReleaser binary from common install locations
- reports the installed `release-tools` version and resolved GoReleaser version
  in `doctor`
- ensures GoReleaser runs from the repository root
- resolves `RELEASE_TOKEN`, a forge-native token variable, or
  `RELEASE_TOKEN_FILE`
- maps the resolved token to the forge-native token environment only for the
  GoReleaser process

This removes environment drift between local shells and CI.

### 4. Release Installers For Consumers

Consumer-facing install scripts can reuse the stable artifact contract to:

- detect OS and CPU architecture
- download the matching binary or archive
- download the checksums file
- verify checksums before install
- install into a configurable directory

This is useful when multiple repos depend on the same CLI or toolkit.

## Key Implementation Decisions

### CLI-Only Public Entrypoint

`release-tools` intentionally keeps the installed CLI as the only public
command surface.

Reason:

- the CLI is explicit and portable across local shells and CI systems
- repo-local `.release-tools.env` replaces Make variables as the config source
- consumers no longer need to copy or include shared Make modules

### Config File With Environment Overrides

The CLI loads `.release-tools.env` from the repository root, unless
`RELEASE_CONFIG_FILE` points elsewhere. Existing environment variables win over
config file values.

Reason:

- committed config documents the release contract
- CI and maintainers can still override values temporarily
- clean tag publishing can pass current config into the temporary tag clone
- release config lives in one small committed file

### Artifact Classes Are Explicit

`RELEASE_ARTIFACTS` records which release artifact classes a repository intends
to use. If unset, the CLI keeps the existing binaries-only behavior. Supported
values are currently `binaries` and `charts`.

Reason:

- binaries-only repositories do not need new config
- chart-aware repositories can opt in before chart-specific config is added
- `doctor` can report the intended release shape early

### Go Preflight Is Optional

The Go CLI requires GoReleaser and only requires a project Go toolchain when
`RELEASE_REQUIRE_GO=1`.

Reason:

- Go CLI projects still can enforce Go availability
- shell/docs/toolkit repos can avoid a project Go preflight while using
  GoReleaser meta archives

GoReleaser itself may still invoke `go` for metadata in some environments. The
dev container includes Go 1.26.4 because this repo now builds the Go CLI.

### Check Versus Snapshot

`release-tools check` runs `goreleaser check`.
`release-tools snapshot` runs `goreleaser release --snapshot --skip=publish --clean`.

Reason:

- `check` is a fast config validation path
- `snapshot` validates the actual artifact pipeline
- neither command requires a publish token

## What To Watch Out For

### Dirty-State Release Failures

GoReleaser can fail if git sees a dirty tree.

Mitigation:

- publish old tags with `release-tools publish-tag`, which uses a clean tag clone

### Wrong Working Directory

If release tooling runs GoReleaser from the caller directory instead of the repo
root, it can inspect the wrong git state.

Mitigation:

- the CLI sets GoReleaser's working directory to the release repository root

### Tag Without Published Release Assets

A git tag alone is not enough for consumer installation.

Mitigation:

- verify that the release page contains uploaded assets after publish
- make installer errors explicit when a tag exists but release assets do not

## Minimal Adoption Checklist For Another Project

1. define artifact names and checksum output in `.goreleaser.yaml`
2. add `.release-tools.env`
3. install `release-tools` into `PATH`
4. call `release-tools version`, `doctor`, `check`, `snapshot`, `publish`, `publish-tag`, and `notes`
5. add CI for push validation and tag publishing if needed
6. add a release installer if downstream repos need binary consumption

## Auth Pattern

For local maintainer use:

```sh
RELEASE_TOKEN_FILE=~/.config/forge/token
```

For CI:

- store `RELEASE_TOKEN` as a repository secret
- let the toolkit map it to `GITEA_TOKEN`, `GITHUB_TOKEN`, or `GITLAB_TOKEN`
  only for the GoReleaser process

## Verification Pattern

This repo uses a dev container as the reproducible test toolbox:

```bash
make container-test
```

For local verification outside the container, use:

```bash
make verify
```

The root `Makefile` is maintainer-only convenience for this repository. Consumer
repositories should install `release-tools` into `PATH` and call that command
from the project root.

When a repository uses its own CLI to release itself, build the current CLI
before publishing and put that build first on `PATH`. Publishing should still go
through the CLI rather than through a Make publish target:

```bash
PATH="$PWD/.tmp:$PATH" release-tools publish-tag vX.Y.Z
```

`scripts/test-errors` verifies the most important CLI failure messages,
including missing release config, missing tag, invalid `GORELEASER_BIN`, and
missing release notes source.

## Recommended Next Improvements

- add integration coverage around `publish-tag` and release body patching
- document consumer-side installer patterns with a concrete example repo
