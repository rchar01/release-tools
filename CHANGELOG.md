# Changelog

All notable changes to `release-tools` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
and adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- required HTTPS for classic Helm package registry uploads so Basic auth tokens
  are not sent over cleartext HTTP
- documented self-release publishing by direct binary path to avoid trusting
  repo-local `.tmp` executables through `PATH`

## [3.4.0] - 2026-07-09

### Added

- added `RELEASE_ARTIFACTS` config parsing and `doctor` reporting for artifact
  classes, defaulting to `binaries` and accepting `binaries` and `charts`
- added local Helm chart validation for chart-enabled repositories, including
  chart directory checks, `helm dependency update --skip-refresh`, `helm lint`,
  and `helm package` into `dist/charts` during snapshots
- added chart packaging before GoReleaser starts `publish` and `publish-tag`,
  with `publish-tag` packaging charts from the clean temporary tag clone
- added `RELEASE_HELM_OCI_REPOSITORY` for pushing packaged charts to a Helm OCI
  repository after GoReleaser publish succeeds
- added explicit Helm OCI registry login support with
  `RELEASE_HELM_OCI_USERNAME`, `RELEASE_HELM_OCI_PASSWORD_FILE`, and
  environment-only `RELEASE_HELM_OCI_PASSWORD`
- added `RELEASE_HELM_OCI_PLAIN_HTTP` for explicit insecure local or disposable
  OCI chart registry tests
- added digest-based OCI chart signing with `RELEASE_HELM_OCI_SIGNER=cosign`,
  optional `RELEASE_HELM_OCI_SIGN_ARGS`, and manifest fields for the
  pushed chart digest and signed digest reference
- added ChartMuseum-compatible classic Helm package uploads, including
  Forgejo/Gitea package registries, with `RELEASE_HELM_CLASSIC_URL`,
  `RELEASE_HELM_CLASSIC_USERNAME`, `RELEASE_HELM_CLASSIC_TOKEN_FILE`, and
  environment-only `RELEASE_HELM_CLASSIC_TOKEN`
- added `make helm-registry-test` for Podman-backed Zot and ChartMuseum smoke
  testing
- added `make helm-oci-signing-test` for Podman-backed Zot and Cosign OCI chart
  signing verification
- added `make helm-provenance-test` for disposable GPG-backed Helm provenance
  signing and `helm verify` smoke testing
- added `make codeberg-smoke-test` for live Codeberg release smoke testing and
  optional Helm package upload checks against a dedicated disposable repository;
  it also verifies manifest asset upload when enabled by the smoke fixture
- added GoReleaser container-image preflights that detect `dockers`,
  `dockers_v2`, `docker_manifests`, and `docker_signs` config and require the
  matching Docker, Podman, Cosign, or configured static signing command during
  `doctor` and `tools-check`
- added `dist/release-manifest.json` for chart-enabled snapshot, publish, and
  publish-tag flows, recording the release tag, chart version, packaged chart
  path, SHA-256, and configured Helm OCI or classic registry targets
- added GoReleaser `dist/artifacts.json` metadata merging into
  `dist/release-manifest.json`, including binary/archive/checksum names, types,
  paths, targets, platforms, and SHA-256 values when available
- added opt-in `RELEASE_MANIFEST_UPLOAD=1` to upload
  `dist/release-manifest.json` as a forge release asset after all configured
  publish-time artifact steps succeed
- added Helm chart provenance signing with `RELEASE_HELM_PROVENANCE`,
  `RELEASE_HELM_GPG_KEY`, and `RELEASE_HELM_GPG_KEYRING`; generated `.prov`
  files are copied back to `dist/charts` and recorded in the release manifest
- added a stable chart release env example and tests that keep documented config
  keys aligned with the CLI allowlist

### Fixed

- kept publish-time Helm chart packages outside GoReleaser's cleaned `dist`
  directory so real `goreleaser release --clean` runs do not delete charts
  before registry upload

## [3.3.0] - 2026-07-02

### Added

- added `RELEASE_NOTES_MODE=gnu-news` for extracting release notes from
  GNU-style `NEWS` files
- added exact generated release-note file tests for both `news-md` and
  `gnu-news` modes

## [3.2.0] - 2026-06-22

### Added

- added `release-tools version` and `release-tools --version` for reporting the
  installed release-tools version
- added resolved GoReleaser version output to `release-tools doctor`
- added first-class docs and tests for `RELEASE_FORGE=codeberg` as a
  Gitea-compatible forge value
- migrated command dispatch to Cobra and Fang for styled help, version flags,
  and generated shell completions
- refreshed the README as a concise landing page with placeholder release
  versions
- documented the `release-tools` self-release procedure for maintainers and
  agents

### Removed

- removed the unused `RELEASE_DOWNLOAD_URL` config key from the public config
  contract
- removed the undocumented `TAG` environment fallback; use `VERSION` or a
  positional tag argument instead

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
