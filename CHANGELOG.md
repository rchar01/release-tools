# Changelog

All notable changes to `release-tools` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
and adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- release-tools brand assets and README logo

## [v1.2.1] - 2026-06-01

### Changed

- clarified the README with the project purpose, values, and what the toolkit
  adds around Goreleaser
- documented release notes generation and release body patching as workflow
  features added around Goreleaser publishing
- documented `v1.2.1` as the current release to pin in consuming repositories

### Fixed

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
