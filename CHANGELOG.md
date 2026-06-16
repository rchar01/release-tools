# Changelog

All notable changes to `release-tools` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
and adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- added `release-tools version` and `release-tools --version` for reporting the
  installed release-tools version
- added resolved GoReleaser version output to `release-tools doctor`
- added first-class docs and tests for `RELEASE_FORGE=codeberg` as a
  Gitea-compatible forge value

## [3.1.0] - 2026-06-12

### Added

- added `RELEASE_TOKEN_FILE` as a supported config key for reading the publish
  token from a local file after environment token variables are checked

## [3.0.0] - 2026-06-12

### Changed

- changed the public forge token contract from `CODEBERG_TOKEN` to
  `RELEASE_TOKEN`
- added `RELEASE_FORGE` with support for `gitea`, `forgejo`, `github`, and
  `gitlab`
- mapped `RELEASE_TOKEN` to `GITEA_TOKEN`, `GITHUB_TOKEN`, or `GITLAB_TOKEN`
  for GoReleaser based on `RELEASE_FORGE`

### Added

- release body patching for GitHub and GitLab releases
- forge-specific default API and download URLs

### Removed

- removed the Codeberg-specific token file fallback from
  `~/.config/codeberg/token`

## [2.2.0] - 2026-06-11

### Changed

- updated the release-tools Go baseline to Go 1.26 with toolchain `go1.26.4`
- updated dev-container verification to install Go `1.26.4` explicitly
- changed release artifacts from toolkit archives to direct OS/arch
  `release-tools` binaries for installation into `PATH`
- changed consumer docs from project-local bootstrap to installed CLI usage
- added maintainer-only Make targets for local and dev-container verification

### Fixed

- changed `publish-tag` to use a full temporary clone and explicit detached
  checkout of `refs/tags/<tag>`, preserving tag history for GoReleaser changelog
  discovery
- removed the shallow-clone warning path from `publish-tag` by preserving tag
  history in the temporary clone

### Removed

- removed legacy private shell helper scripts from `bin/`; the Go CLI is now the
  only release implementation path
- removed the project-local bootstrap script and `bin/release-tools` wrapper
  from the consumer model

## [2.1.0] - 2026-06-11

### Added

- compiled Go implementation of the `release-tools` CLI while preserving the
  v2 command and `.release-tools.env` contract
- Go unit tests for config parsing, version argument validation, and NEWS.md
  release note extraction

### Changed

- changed self-release artifacts from a meta archive to OS/arch-specific
  toolkit archives containing the compiled `release-tools` binary
- updated the bootstrap example to download release archives before falling back
  to a pinned git checkout

## [2.0.0] - 2026-06-11

### Added

- `bin/release-tools` as the sole public command surface for tool checks,
  doctor, check, snapshot, publish, publish-tag, and notes commands
- `.release-tools.env` config loading with environment overrides
- self-release support through `.release-tools.env` and `.goreleaser.yaml`
- dev-container verification scripts for CLI behavior and error messages
- release-tools brand assets and README logo

### Changed

- changed `release-tools check` to run `goreleaser check`
- made Go optional unless `RELEASE_REQUIRE_GO=1` is set
- updated docs from Make-only runtime bootstrap to CLI-only runtime bootstrap

### Removed

- shared Make wrapper and Make-based consumer integration example

## [1.2.1] - 2026-06-01

### Changed

- clarified the README with the project purpose, values, and what the toolkit
  adds around Goreleaser
- documented release notes generation and release body patching as workflow
  features added around Goreleaser publishing
- documented `v1.2.1` as the current release to pin in consuming repositories

## [1.2.0] - 2026-04-15

### Added

- consumer integration guide for Make-only runtime-bootstrap repositories
- ready-to-copy examples for bootstrap, Makefile integration, version pinning, and Forgejo release CI
- docs index for the `docs/` directory

### Changed

- consolidated duplicate consumer setup docs into `docs/usage.md`
- clarified the public integration contract and runtime-bootstrap usage across the repo docs

## [1.1.0] - 2026-04-15

### Changed

- removed the Just frontend and standardized the shared interface on Make
- changed `release-tag` to use the current bootstrapped toolkit against a clean tag clone via `RELEASE_REPO_ROOT`
- switched the default toolkit path in the shared Make frontend to `.tmp/release-tools/current`

## [1.0.0] - 2026-04-15

### Added

- reusable agent guide for the Go release flow pattern used by this toolkit

### Changed

- renamed the public token contract to `CODEBERG_TOKEN`
- cleaned up the shared release frontends and tightened token handling for GoReleaser

## [0.0.1] - 2026-04-15

### Added

- initial `release-tools` release with shared GoReleaser-based release automation
