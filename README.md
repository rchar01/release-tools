<div align="center">
  <img src="assets/brand/release-tools-forge-avatar-transparent-512.png" width="256" alt="release-tools logo">
</div>

<h1 align="center">release-tools</h1>

<p align="center">
  A small installed CLI for standardizing GoReleaser-based release workflows
  across repositories.
</p>

---

`release-tools` keeps project-specific build config in each repository while
moving repeatable release behavior into one command on `PATH`.

It is intended for Go, shell, and documentation/toolkit projects that publish
with GoReleaser on Codeberg, Gitea, Forgejo, GitHub, or GitLab.

## Overview

GoReleaser still owns builds, checksums, archives, and release asset publishing.
`release-tools` adds the workflow around it:

- stable commands for checking, snapshotting, publishing, and generating notes
- repo-local `.release-tools.env` configuration with environment overrides
- a shared `RELEASE_TOKEN` / `RELEASE_TOKEN_FILE` token contract
- forge-aware token mapping for GoReleaser
- release notes generation from `NEWS.md`
- optional release body patching after publish
- safe `publish-tag` releases from a clean clone of the exact tag

Consumer repositories install the CLI once and run `release-tools` directly from
the project root. They should not copy release helper scripts or use this repo's
Makefile as a release frontend.

## Requirements

Consumer repositories need:

- the `release-tools` binary installed on `PATH`
- GoReleaser available on `PATH` or through `GORELEASER_BIN`
- a repo-local `.release-tools.env`
- a project-owned `.goreleaser.yaml`
- `NEWS.md` when `RELEASE_NOTES_MODE=news-md`

Maintainers of this repository also need Go `1.26`, Make, and Podman for the
container verification path.

## Installation

Download the matching release binary from Codeberg and place it in a directory on
`PATH`.

Linux amd64 example:

```bash
RELEASE_TOOLS_VERSION=vX.Y.Z
version="${RELEASE_TOOLS_VERSION#v}"

mkdir -p "$HOME/.local/bin"
curl -fsSL -o "$HOME/.local/bin/release-tools" \
  "https://codeberg.org/rch/release-tools/releases/download/${RELEASE_TOOLS_VERSION}/release-tools_${version}_linux_amd64"
chmod +x "$HOME/.local/bin/release-tools"
```

Published binary names use this shape:

```text
release-tools_<version>_<os>_<arch>
```

Supported assets:

- `release-tools_X.Y.Z_linux_amd64`
- `release-tools_X.Y.Z_linux_arm64`
- `release-tools_X.Y.Z_darwin_amd64`
- `release-tools_X.Y.Z_darwin_arm64`
- `checksums.txt`

## Quick Start

Add `.release-tools.env` to the repository that will use `release-tools`:

```sh
RELEASE_PROJECT=mycli
RELEASE_FORGE=codeberg
RELEASE_OWNER=myowner
RELEASE_REPO=mycli
# RELEASE_TOKEN_FILE=~/.config/forge/token
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml
```

The consuming repository also owns its `.goreleaser.yaml` and `NEWS.md`.

Run local checks from the consumer repository root:

```bash
release-tools version
release-tools doctor
release-tools check
release-tools snapshot
release-tools notes v1.2.3
```

Publish an existing tag with an available token:

```bash
release-tools publish-tag v1.2.3
```

Publishing requires `RELEASE_TOKEN`, the native GoReleaser token variable for
the selected forge, or `RELEASE_TOKEN_FILE` pointing at a local token file.

## Commands

| Command | Purpose |
| --- | --- |
| `release-tools version` | Print the installed `release-tools` version. |
| `release-tools doctor` | Validate release config and required tools. |
| `release-tools check` | Run `goreleaser check`. |
| `release-tools snapshot` | Run a local snapshot build without publishing. |
| `release-tools publish` | Publish the current tag from the current worktree. |
| `release-tools publish-tag vX.Y.Z` | Publish a specific existing tag from a clean clone. |
| `release-tools notes vX.Y.Z` | Generate release notes for a tag. |
| `release-tools completion <shell>` | Generate shell completions. |

Supported completion shells are `bash`, `zsh`, `fish`, and `powershell`:

```bash
release-tools completion bash
release-tools completion zsh
release-tools completion fish
release-tools completion powershell
```

## Configuration

Supported `RELEASE_FORGE` values are:

- `codeberg`
- `gitea`
- `forgejo`
- `github`
- `gitlab`

`codeberg`, `gitea`, and `forgejo` use Codeberg-compatible defaults unless
`RELEASE_API_URL` is set explicitly.

Required for release commands:

- `RELEASE_PROJECT`
- `RELEASE_OWNER`

Additionally required for `publish-tag`:

- `VERSION` or a positional tag argument such as `v1.2.3`

For the full public config contract, token resolution rules, and consumer setup
guide, see [`docs/usage.md`](docs/usage.md).

## Examples And Docs

Ready-to-copy consumer starting points:

- [`examples/.release-tools.env`](examples/.release-tools.env)
- [`examples/forgejo-release.yml`](examples/forgejo-release.yml)

Documentation:

- [`docs/README.md`](docs/README.md): docs index
- [`docs/usage.md`](docs/usage.md): consumer integration contract
- [`docs/agent-release-flow.md`](docs/agent-release-flow.md): release-flow
  rationale and maintainer/agent notes

## Maintainer Workflow

The root `Makefile` is for maintainers working on this repository. It is not
part of the consumer integration contract.

Common maintainer commands:

```bash
make verify
make container-test
make build
make check
make snapshot
make clean
```

## Testing

Run the local verification suite:

```bash
make verify
```

Run the same checks inside the dev container:

```bash
make container-test
```

For focused CLI error-message checks, use:

```bash
scripts/test-errors
```

## Self-Release

`release-tools` releases itself with the `release-tools` CLI. The Makefile is
only used for verification and for building a current local binary when the
globally installed CLI may not include unreleased behavior yet.

Before tagging, run verification and update `NEWS.md` and `CHANGELOG.md` from
`Unreleased` to the release version:

```bash
make verify
make container-test
```

After committing and pushing the release prep, create and push the tag:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push cb main vX.Y.Z
```

Build the current CLI and publish from the exact tag:

```bash
make build
PATH="$PWD/.tmp:$PATH" release-tools publish-tag vX.Y.Z
```

This keeps `release-tools` as the publishing frontend while avoiding reliance on
an older installed binary during self-release bootstrapping.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for
details.
