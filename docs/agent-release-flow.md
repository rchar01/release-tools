# Agent Guide: Go Release Flow Pattern

This document is written for other agents and maintainers who want to reuse the
release toolkit pattern from `release-tools` in another Go project.

## Goal

Build a Go CLI release flow that is:

- reproducible
- safe to run locally
- safe to run in CI
- compatible with Codeberg and Forgejo/Gitea releases
- easy for consumer repositories to install from published artifacts

## Pattern Summary

The pattern used by this toolkit has seven parts:

1. Goreleaser builds cross-platform archives and checksums
2. local helpers provide stable commands through `make`
3. a shared tagged `release-tools` checkout owns release behavior
4. a tagged release can be published from a clean temporary clone
5. consumers install from release assets, not from source builds
6. release page text can be generated from a short repo-local notes source such as `NEWS.md`
7. generated release notes should be body-only Markdown because the forge already renders the release title from the tag

## Core Files To Recreate

- `.goreleaser.yaml`
- `.forgejo/workflows/ci.yml`
- `.forgejo/workflows/release.yml`
- `scripts/bootstrap-release-tools.sh`
- `.tmp/release-tools/current/make/release-tools.mk`
- repo-local install script for release assets if consumers need binary installs
- `Makefile`

## Why This Pattern Works

### 1. Stable artifact contract

Goreleaser defines the exact supported OS/arch matrix and archive names.

Benefits:

- consumers know exactly what to download
- checksums are generated consistently
- local snapshot builds resemble real releases

### 2. Safe local publishing

Publishing from a maintainer worktree is risky when:

- `main` is ahead of the tag
- the worktree is dirty
- helper scripts have changed after the tagged commit

`release-tag.sh` avoids that by cloning the tag from the repo git database and
running the current bootstrapped toolkit against that clean clone.

Benefits:

- the published release always matches the tag
- local uncommitted changes cannot leak into the release

### 3. Shared Goreleaser entrypoint

`release-tools/bin/run-goreleaser.sh` does three useful things:

- resolves the Goreleaser binary from common install locations
- ensures Goreleaser runs from the repository root
- keeps the release entrypoint identical across consuming repos

This removes environment drift between local shells and CI.

### 4. Release installers for consumers

Consumer-facing install scripts can reuse the stable artifact contract to:

- detect OS and CPU architecture
- download the matching archive
- download the checksums file
- verify checksums before install
- install into a configurable directory

This is useful when multiple repos depend on the same CLI.

## Key Implementation Decisions

### Snapshot validation instead of remote-bound checks

`release-check` uses a snapshot build, not `goreleaser check`.

Reason:

- snapshot builds validate the real artifact pipeline
- they do not depend on configured remotes or a published tag

### Make frontend validates required variables

The shared Make frontend checks required repo-specific variables before running
release commands.

Reason:

- missing `RELEASE_PROJECT` or `RELEASE_OWNER` should fail fast in the frontend
- `release-tag` should fail early when `VERSION` is not set

### Repo-local Go temp/cache directories

The Goreleaser `before` hook uses repo-local `.tmp` directories.

Reason:

- some systems mount `/tmp` with `noexec`
- `go test` can fail in those environments if the default temp directory is used

### Archives contain only the binary

The archive config should explicitly avoid packaging extra docs unless the
consumer contract requires them.

Reason:

- consumer automation often expects a single root-level binary
- packaging extra files can break that contract

## What To Watch Out For

### Dirty-state release failures

Goreleaser will fail if git sees a dirty tree.

Mitigation:

- publish from a clean tag clone

### Wrong working directory

If helper scripts run Goreleaser from the caller directory instead of the repo
root, it can inspect the wrong git state.

Mitigation:

- force `cd "$REPO_ROOT"` before execing Goreleaser

### Tag without published release assets

A git tag alone is not enough for consumer installation.

Mitigation:

- verify that the release page contains uploaded assets after publish
- make installer errors explicit when a tag exists but release assets do not

### Tag clone using the wrong toolkit revision

If `release-tag` uses the wrong pinned toolkit revision, the published behavior
can drift from the caller's intended release flow.

Mitigation:

- resolve the toolkit version from repo config or explicit override
- bootstrap the exact pinned toolkit checkout before invoking the shared Make frontend
- run the bootstrapped toolkit scripts against `RELEASE_REPO_ROOT=<clean tag clone>` rather than expecting toolkit files inside the tag checkout

## Minimal Adoption Checklist For Another Go Project

1. define your OS/arch matrix in `.goreleaser.yaml`
2. define exact archive names and checksum output
3. add `make` targets for test, release-check, release-snapshot, release, and release-tag
4. pin the `release-tools` tag in repo-local config
5. add a bootstrap script that checks out the pinned toolkit into `.tmp/release-tools/current`
6. include `.tmp/release-tools/current/make/release-tools.mk`
7. add CI for push validation and tag publishing
8. add a release installer if downstream repos need binary consumption

## Auth Pattern

For local maintainer use:

```bash
export CODEBERG_TOKEN="$(cat ~/.config/codeberg/token)"
```

For CI:

- store `CODEBERG_TOKEN` as a repository secret
- let the toolkit map it to `GITEA_TOKEN` only for the Goreleaser process

## Recommended Next Improvements For The Toolkit

- add integration coverage around `release-tag` and release body patching
- document consumer-side installer patterns with a concrete example repo
- consider adding an installer helper for projects that want a shared binary download flow
