# News

This file gives a short, release-oriented view of what changed between versions.

## Unreleased

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
