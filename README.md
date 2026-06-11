# release-tools

<div align="center">
  <img src="assets/brand/release-tools-forge-avatar-transparent-512.png" width="256" alt="release-tools logo">
</div>

Shared release automation for Go and shell-toolkit repositories using
Goreleaser, Codeberg, and a small CLI.

## Purpose

`release-tools` is a small shared release layer around Goreleaser. It keeps
project-specific build configuration in each consuming repository while moving
repeatable release behavior into one pinned toolkit.

Use it when you want multiple projects to share the same release command
surface, token convention, validation, release notes flow, and safe tag-publish
behavior without copying release scripts between repositories.

## What It Adds Over Goreleaser

Goreleaser still owns builds, archives, checksums, and release asset publishing.
This toolkit adds the workflow around it:

- stable CLI commands such as `check`, `snapshot`, `publish`, `publish-tag`,
  `notes`, and `doctor`
- repo-local `.release-tools.env` configuration with environment overrides
- pinned runtime bootstrapping through `.tmp/release-tools/current`
- fast validation for required release variables such as `RELEASE_PROJECT`,
  `RELEASE_OWNER`, and `VERSION`
- a public `CODEBERG_TOKEN` contract that is mapped internally to Goreleaser's
  `GITEA_TOKEN`
- safer `publish-tag` publishing from a clean temporary clone of the exact tag
- consistent Goreleaser execution from the repository root
- release notes generation from `NEWS.md`, passed into Goreleaser during
  publish, with optional release body patching after publish

## Values

- reproducible releases from a pinned toolkit version
- minimal release logic in consuming repositories
- safe local and CI publishing paths
- clear separation between shared behavior and project-specific configuration
- small CLI commands that are easy for humans, CI, and agents to run

Repos should consume this toolkit from a pinned runtime checkout such as
`.tmp/release-tools/current` and call `bin/release-tools` from that checkout.

Current release to pin in consuming repositories:

```text
v2.0.0
```

The CLI validates required release configuration before running release commands.
Consumers should set at least `RELEASE_PROJECT` and `RELEASE_OWNER`, either in
`.release-tools.env` or the environment. Tag publishing also requires `VERSION`
or a positional tag.

## Consumer Quickstart

In a consuming repository:

1. Copy `examples/bootstrap-release-tools.sh` to `scripts/bootstrap-release-tools.sh`.
2. Copy `examples/.release-tools.env` to `.release-tools.env` and set the project values.
3. Add or update the project's `.goreleaser.yaml` and `NEWS.md`.
4. Bootstrap the pinned toolkit and run the CLI from that checkout.

Minimal `.release-tools.env` shape:

```sh
RELEASE_PROJECT=mycli
RELEASE_TOOLS_VERSION=v2.0.0
RELEASE_OWNER=myowner
RELEASE_REPO=mycli
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml
```

Common local verification flow:

```bash
toolkit_dir="$(./scripts/bootstrap-release-tools.sh)"
"$toolkit_dir/bin/release-tools" doctor
"$toolkit_dir/bin/release-tools" check
"$toolkit_dir/bin/release-tools" snapshot
"$toolkit_dir/bin/release-tools" notes v1.2.3
```

Publishing requires `CODEBERG_TOKEN` and an existing tag:

```bash
export CODEBERG_TOKEN="$(cat ~/.config/codeberg/token)"
"$toolkit_dir/bin/release-tools" publish-tag v1.2.3
```

See `docs/usage.md` for the full integration contract.

CLI commands from a bootstrapped checkout:

```bash
.tmp/release-tools/current/bin/release-tools doctor
.tmp/release-tools/current/bin/release-tools check
.tmp/release-tools/current/bin/release-tools snapshot
.tmp/release-tools/current/bin/release-tools publish
.tmp/release-tools/current/bin/release-tools publish-tag v1.2.3
.tmp/release-tools/current/bin/release-tools notes v1.2.3
```

This repository also ships ready-to-copy consumer examples for the runtime
bootstrap flow:

- `docs/README.md`
- `examples/bootstrap-release-tools.sh`
- `examples/.release-tools.env`
- `examples/forgejo-release.yml`

See these docs:

- `docs/README.md` for the docs index
- `docs/usage.md` for the integration contract and consumer setup guide
- `docs/agent-release-flow.md` for the reusable release pattern and rationale
