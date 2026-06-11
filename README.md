# release-tools

<div align="center">
  <img src="assets/brand/release-tools-forge-avatar-transparent-512.png" width="256" alt="release-tools logo">
</div>

Shared release automation for Go and shell-toolkit repositories using
Goreleaser, Codeberg, and a small installed Go CLI.

## Purpose

`release-tools` is a small shared release layer around Goreleaser. It keeps
project-specific build configuration in each consuming repository while moving
repeatable release behavior into one installed helper command.

Use it when you want multiple projects to share the same release command
surface, token convention, validation, release notes flow, and safe tag-publish
behavior without copying release scripts between repositories.

## What It Adds Over Goreleaser

Goreleaser still owns builds, archives, checksums, and release asset publishing.
This toolkit adds the workflow around it:

- stable CLI commands such as `check`, `snapshot`, `publish`, `publish-tag`,
  `notes`, and `doctor`
- repo-local `.release-tools.env` configuration with environment overrides
- fast validation for required release variables such as `RELEASE_PROJECT`,
  `RELEASE_OWNER`, and `VERSION`
- a public `CODEBERG_TOKEN` contract that is mapped internally to Goreleaser's
  `GITEA_TOKEN`
- safer `publish-tag` publishing from a clean temporary clone of the exact tag
- consistent Goreleaser execution from the repository root
- release notes generation from `NEWS.md`, passed into Goreleaser during
  publish, with optional release body patching after publish

## Values

- one small command installed in the user's `PATH`
- minimal release logic in consuming repositories
- safe local and CI publishing paths
- clear separation between shared behavior and project-specific configuration
- small CLI commands that are easy for humans, CI, and agents to run

Install `release-tools` once into a directory on `PATH`, then run it directly
from each project repository.

The CLI validates required release configuration before running release commands.
Consumers should set at least `RELEASE_PROJECT` and `RELEASE_OWNER`, either in
`.release-tools.env` or the environment. Tag publishing also requires `VERSION`
or a positional tag.

## Consumer Quickstart

Install the CLI:

```bash
mkdir -p "$HOME/.local/bin"
curl -fsSL -o "$HOME/.local/bin/release-tools" \
  "https://codeberg.org/rch/release-tools/releases/download/v2.2.0/release-tools_2.2.0_linux_amd64"
chmod +x "$HOME/.local/bin/release-tools"
```

Use the matching binary for your OS and architecture:

- `release-tools_2.2.0_linux_amd64`
- `release-tools_2.2.0_linux_arm64`
- `release-tools_2.2.0_darwin_amd64`
- `release-tools_2.2.0_darwin_arm64`

In a project repository:

1. Copy `examples/.release-tools.env` to `.release-tools.env` and set the project values.
2. Add or update the project's `.goreleaser.yaml` and `NEWS.md`.
3. Run `release-tools` from the project root.

Minimal `.release-tools.env` shape:

```sh
RELEASE_PROJECT=mycli
RELEASE_OWNER=myowner
RELEASE_REPO=mycli
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml
```

Common local verification flow:

```bash
release-tools doctor
release-tools check
release-tools snapshot
release-tools notes v1.2.3
```

Publishing requires `CODEBERG_TOKEN` and an existing tag:

```bash
export CODEBERG_TOKEN="$(cat ~/.config/codeberg/token)"
release-tools publish-tag v1.2.3
```

See `docs/usage.md` for the full integration contract.

This repository also ships ready-to-copy consumer examples for installed CLI
usage:

- `docs/README.md`
- `examples/.release-tools.env`
- `examples/forgejo-release.yml`

See these docs:

- `docs/README.md` for the docs index
- `docs/usage.md` for the integration contract and consumer setup guide
- `docs/agent-release-flow.md` for the reusable release pattern and rationale

## Maintainer Workflow

This repository includes a root `Makefile` for maintainers and agents working on
`release-tools` itself. It is not part of the consumer integration contract.

Use these targets instead of invoking lower-level scripts directly:

```bash
make verify
make container-test
make check
make snapshot
make clean
```

Consumers should install `release-tools` into `PATH` and run that command from
their project root.
