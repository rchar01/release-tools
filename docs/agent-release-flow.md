# Agent Guide: CLI Release Flow Pattern

This document is written for agents and maintainers who want to reuse the
release toolkit pattern from `release-tools` in another project.

## Goal

Build a release flow that is:

- reproducible
- safe to run locally
- safe to run in CI
- compatible with Codeberg and Forgejo/Gitea releases
- easy for consumer repositories to install from published artifacts
- driven by one CLI entrypoint

## Pattern Summary

The pattern used by this toolkit has seven parts:

1. GoReleaser builds or packages release artifacts and checksums
2. `bin/release-tools` provides the stable command surface
3. `.release-tools.env` stores project-specific release configuration
4. a shared tagged `release-tools` checkout owns release behavior
5. a tagged release can be published from a clean temporary clone
6. consumers install from release assets, not from source builds
7. release page text can be generated from a short repo-local notes source such as `NEWS.md`

## Core Files To Recreate

- `.release-tools.env`
- `.goreleaser.yaml`
- `scripts/bootstrap-release-tools.sh`
- `.tmp/release-tools/current/bin/release-tools`
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

For Go CLIs this usually means OS/arch-specific binary archives. For shell or
documentation toolkits this can be a meta/source archive with no Go build.

### 2. Safe Local Publishing

Publishing from a maintainer worktree is risky when:

- `main` is ahead of the tag
- the worktree is dirty
- helper scripts have changed after the tagged commit

`release-tag.sh` avoids that by cloning the tag from the repo git database and
running the current bootstrapped toolkit against that clean clone.

Benefits:

- the published release always matches the tag
- local uncommitted changes cannot leak into the release

### 3. Shared GoReleaser Entrypoint

`release-tools/bin/release-tools` provides the stable command surface, and
`release-tools/bin/run-goreleaser.sh` does three useful things:

- resolves the GoReleaser binary from common install locations
- ensures GoReleaser runs from the repository root
- maps `CODEBERG_TOKEN` to `GITEA_TOKEN` only for the GoReleaser process

This removes environment drift between local shells and CI.

### 4. Release Installers For Consumers

Consumer-facing install scripts can reuse the stable artifact contract to:

- detect OS and CPU architecture
- download the matching archive
- download the checksums file
- verify checksums before install
- install into a configurable directory

This is useful when multiple repos depend on the same CLI or toolkit.

## Key Implementation Decisions

### CLI-Only Public Entrypoint

`release-tools` v2 intentionally removes the Make wrapper from the public
contract.

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
- the toolkit version pin lives beside the rest of the release configuration

### Go Preflight Is Optional

`bin/ensure-tools.sh` requires GoReleaser and only requires Go when
`RELEASE_REQUIRE_GO=1`.

Reason:

- Go CLI projects still can enforce Go availability
- shell/docs/toolkit repos can avoid a release-tools Go preflight while using
  GoReleaser meta archives

GoReleaser itself may still invoke `go` for metadata in some environments, so
the dev container includes Go.

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

If helper scripts run GoReleaser from the caller directory instead of the repo
root, it can inspect the wrong git state.

Mitigation:

- force `cd "$REPO_ROOT"` before execing GoReleaser

### Tag Without Published Release Assets

A git tag alone is not enough for consumer installation.

Mitigation:

- verify that the release page contains uploaded assets after publish
- make installer errors explicit when a tag exists but release assets do not

### Tag Clone Using The Wrong Toolkit Revision

If `publish-tag` uses the wrong pinned toolkit revision, published behavior can
drift from the caller's intended release flow.

Mitigation:

- resolve the toolkit version from repo config or explicit override
- bootstrap the exact pinned toolkit checkout before invoking the CLI
- run the bootstrapped toolkit scripts against `RELEASE_REPO_ROOT=<clean tag clone>`

## Minimal Adoption Checklist For Another Project

1. define artifact names and checksum output in `.goreleaser.yaml`
2. add `.release-tools.env`
3. set `RELEASE_TOOLS_VERSION` in `.release-tools.env`
4. add a bootstrap script that checks out the pinned toolkit into `.tmp/release-tools/current`
5. call `bin/release-tools doctor`, `check`, `snapshot`, `publish`, `publish-tag`, and `notes`
6. add CI for push validation and tag publishing if needed
7. add a release installer if downstream repos need binary consumption

## Auth Pattern

For local maintainer use:

```bash
export CODEBERG_TOKEN="$(cat ~/.config/codeberg/token)"
```

For CI:

- store `CODEBERG_TOKEN` as a repository secret
- let the toolkit map it to `GITEA_TOKEN` only for the GoReleaser process

## Verification Pattern

This repo uses a dev container as the reproducible test toolbox:

```bash
./scripts/in-container ./scripts/test
```

`scripts/test-errors` verifies the most important CLI failure messages,
including missing release config, missing tag, invalid `GORELEASER_BIN`, and
missing release notes source.

## Recommended Next Improvements

- add integration coverage around `publish-tag` and release body patching
- document consumer-side installer patterns with a concrete example repo
- consider adding an installer helper for projects that want a shared binary download flow
- consider moving more implementation details behind the CLI once the shell
  command surface is proven stable
