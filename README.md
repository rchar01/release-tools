# release-tools

Shared release automation for Go repositories using Goreleaser, Codeberg, and
Make.

## Purpose

`release-tools` is a small shared release layer around Goreleaser. It keeps
project-specific build configuration in each consuming repository while moving
repeatable release behavior into one pinned toolkit.

Use it when you want multiple Go projects to share the same release command
surface, token convention, validation, release notes flow, and safe tag-publish
behavior without copying shell scripts between repositories.

## What It Adds Over Goreleaser

Goreleaser still owns builds, archives, checksums, and release asset publishing.
This toolkit adds the workflow around it:

- stable Make targets such as `release-check`, `release-snapshot`, `release`,
  `release-tag`, and `release-notes`
- pinned runtime bootstrapping through `.tmp/release-tools/current`
- fast validation for required release variables such as `RELEASE_PROJECT`,
  `RELEASE_OWNER`, and `VERSION`
- a public `CODEBERG_TOKEN` contract that is mapped internally to Goreleaser's
  `GITEA_TOKEN`
- safer `release-tag` publishing from a clean temporary clone of the exact tag
- consistent Goreleaser execution from the repository root
- optional release notes and release body helpers

## Values

- reproducible releases from a pinned toolkit version
- minimal release logic in consuming repositories
- safe local and CI publishing paths
- clear separation between shared behavior and project-specific configuration
- small, Make-first commands that are easy for humans and agents to run

Repos should consume this toolkit from a pinned runtime checkout such as
`.tmp/release-tools/current` and include `make/release-tools.mk` from there.

Current release to pin in consuming repositories:

```text
v1.2.1
```

The shared Make frontend validates required release configuration before running
release commands. Consumers should set at least `RELEASE_PROJECT` and
`RELEASE_OWNER`, and `release-tag` also requires `VERSION`.

This repository also ships ready-to-copy consumer examples for the runtime
bootstrap flow:

- `docs/README.md`
- `examples/bootstrap-release-tools.sh`
- `examples/Makefile.release-tools`
- `examples/.release-tools-version`
- `examples/forgejo-release.yml`

See these docs:

- `docs/README.md` for the docs index
- `docs/usage.md` for the integration contract and consumer setup guide
- `docs/agent-release-flow.md` for the reusable release pattern and rationale
