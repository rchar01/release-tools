# News

This file gives a short, release-oriented view of what changed between versions.

## Unreleased

## v3.1.0 - 2026-06-12

- add `RELEASE_TOKEN_FILE` so local maintainers can configure a token file path
  instead of exporting `RELEASE_TOKEN` manually before publishing

## v3.0.0 - 2026-06-12

- breaking: replace the Codeberg-specific `CODEBERG_TOKEN` contract with
  `RELEASE_FORGE` and `RELEASE_TOKEN`
- map `RELEASE_TOKEN` to `GITEA_TOKEN`, `GITHUB_TOKEN`, or `GITLAB_TOKEN` for
  GoReleaser based on the selected forge
- add release body patching for GitHub and GitLab in addition to Gitea/Forgejo
- keep Codeberg-compatible defaults for `RELEASE_FORGE=gitea`, while allowing
  explicit `RELEASE_API_URL` and `RELEASE_DOWNLOAD_URL` overrides

## v2.2.0 - 2026-06-11

- make `publish-tag` clone full tag history before detaching at the requested
  tag, so GoReleaser can discover previous tags and avoid shallow-clone warnings
- update the release-tools Go baseline to Go 1.26 with toolchain `go1.26.4`
- publish direct OS/arch `release-tools` binaries for installation into `PATH`
- remove the project-local bootstrap and `bin/` wrapper model from consumer docs
- remove legacy private shell helper scripts from `bin/`; the Go CLI is now the
  only release implementation path
- add maintainer-only Make targets for local and dev-container verification of
  this repository

## v2.1.0 - 2026-06-11

- add a compiled Go implementation of the `release-tools` CLI while preserving
  the v2 command and `.release-tools.env` contract
- publish OS/arch-specific toolkit archives that include the compiled CLI,
  docs, examples, scripts, and release metadata
- update the bootstrap script to download release archives before falling back to
  a pinned git checkout
- add Go unit tests for config parsing, version handling, and NEWS.md release
  note extraction

## v2.0.0 - 2026-06-11

- switch to a CLI-only release workflow with `bin/release-tools` as the sole
  public entrypoint
- add `.release-tools.env` config loading with environment overrides
- add self-release support for `release-tools` using a GoReleaser meta archive
- make Go optional unless `RELEASE_REQUIRE_GO=1` is set
- add dev-container verification scripts for CLI behavior and error messages
- remove the shared Make wrapper from the public integration model
- add release-tools brand assets and show the logo in the README

## v1.2.1 - 2026-06-01

- clarify the README with the project purpose, values, and what the toolkit adds
  around Goreleaser
- call out release notes generation and release body patching as workflow
  features added around Goreleaser publishing
- document `v1.2.1` as the current release to pin in consuming repositories

## v1.2.0 - 2026-04-15

- add a consumer integration guide and ready-to-copy examples for Make-only runtime bootstrap
- document the bootstrapped toolkit model and consumer-facing integration contract more clearly

## v1.1.0 - 2026-04-15

- drop the Just frontend and standardize on the Make-only bootstrapped checkout model
- update release publishing so `release-tag` runs the current bootstrapped toolkit against a clean tag clone

## v1.0.0 - 2026-04-15

- rename the public release token contract to `CODEBERG_TOKEN`
- clean up the shared frontends and tighten GoReleaser token handling
- add a reusable agent guide for the Go release flow pattern used by this toolkit

## v0.0.1 - 2026-04-15

- initial `release-tools` release with shared GoReleaser-based release automation
