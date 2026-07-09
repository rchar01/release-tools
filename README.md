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
- a shared `RELEASE_TOKEN` environment token contract
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
- Docker, Podman, [Cosign](https://github.com/sigstore/cosign), or another
  configured signing command when the project-owned GoReleaser config uses
  container image or image-signing pipes
- Helm available on `PATH` when `RELEASE_ARTIFACTS` includes `charts`
- a repo-local `.release-tools.env`
- a project-owned `.goreleaser.yaml`
- `NEWS.md` when release notes are generated from a notes source

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
- `release-tools_X.Y.Z_darwin_amd64`
- `checksums.txt`

## Quick Start

Add `.release-tools.env` to the repository that will use `release-tools`:

```sh
RELEASE_PROJECT=mycli
RELEASE_FORGE=codeberg
RELEASE_OWNER=myowner
RELEASE_REPO=mycli
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml
```

The consuming repository also owns its `.goreleaser.yaml` and `NEWS.md`.
Set `RELEASE_NOTES_MODE=news-md` for Markdown headings such as
`## v1.2.3 - 2026-07-02`, or `RELEASE_NOTES_MODE=gnu-news` for GNU-style
release headings such as `* Noteworthy changes in release 1.2.3 (2026-07-02)`.

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
the selected forge, or environment-only `RELEASE_TOKEN_FILE` pointing at a local
token file.

Validation commands (`check` and `snapshot`) do not read token files and strip
release-token variables before invoking GoReleaser.

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

Consumer repositories configure `release-tools` with `.release-tools.env` and
environment overrides. The Quick Start example shows the common minimum shape.

The canonical public contract for supported keys, environment-only variables,
token resolution, release-notes modes, artifact classes, Helm chart behavior,
release manifests, and container-image preflights is
[`docs/usage.md`](docs/usage.md).

## Examples And Docs

Ready-to-copy consumer starting points:

- [`examples/.release-tools.env`](examples/.release-tools.env)
- [`examples/chart-release.env`](examples/chart-release.env)
- [`examples/forgejo-release.yml`](examples/forgejo-release.yml)

Use `examples/chart-release.env` by copying it into the consumer repository as
`.release-tools.env`, or set `RELEASE_CONFIG_FILE` explicitly when using another
file name.

Documentation:

- [`docs/README.md`](docs/README.md): docs index
- [`docs/usage.md`](docs/usage.md): consumer integration contract
- [`docs/agent-release-flow.md`](docs/agent-release-flow.md): release-flow
  rationale and maintainer/agent notes
- [`docs/future-work.md`](docs/future-work.md): deferred ideas and intentionally
  out-of-scope directions

## Maintainer Workflow

The root `Makefile` is for maintainers working on this repository. It is not
part of the consumer integration contract.

Common maintainer commands:

```bash
make verify
make container-test
make helm-registry-test
make helm-oci-signing-test
make helm-provenance-test
make codeberg-smoke-test
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

The dev container verifies pinned SHA-256 checksums before installing downloaded
Go, GoReleaser, Helm, and Cosign release artifacts.

Run Podman-backed Helm registry smoke tests against local Zot and ChartMuseum
containers:

```bash
make helm-registry-test
```

Run a Podman-backed Helm OCI signing smoke test against local Zot. This builds
the current CLI, generates a temporary Cosign key pair, signs the pushed chart by
digest, and verifies the signature from a clean environment:

```bash
make helm-oci-signing-test
```

This target requires a trusted
[`cosign`](https://github.com/sigstore/cosign) on `PATH`; the dev container
includes Cosign for containerized verification.

For a local host install, use the upstream Cosign release binary or your trusted
package manager. Linux amd64 example:

```bash
curl -O -L "https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64"
sudo mv cosign-linux-amd64 /usr/local/bin/cosign
sudo chmod +x /usr/local/bin/cosign

cosign version
```

Run a disposable GPG-backed Helm provenance smoke test. This builds the current
CLI, generates a temporary signing key, runs chart-enabled `release-tools
snapshot`, and verifies the signed chart with `helm verify`:

```bash
make helm-provenance-test
```

Run the live Codeberg smoke test against `rch/release-tools-smoke` with a token
that can push to that repository and create releases. The smoke verifies release
body patching and manifest asset upload. Package-registry access is optional and
enables the Helm upload portion of the smoke test:

```bash
make codeberg-smoke-test
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

Also run `make helm-registry-test` for Helm registry publishing changes,
`make helm-oci-signing-test` for Helm OCI signing changes, and
`make helm-provenance-test` for Helm provenance signing changes.

After committing and pushing the release prep, create and push the tag:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push cb main vX.Y.Z
```

Build the current CLI and publish from the exact tag:

```bash
make build
./.tmp/release-tools publish-tag vX.Y.Z
```

This keeps `release-tools` as the publishing frontend while avoiding reliance on
an older installed binary during self-release bootstrapping. Invoke the binary by
path instead of prepending `.tmp` to `PATH`, so child tool resolution does not
trust every executable in the repo-local build directory during publishing.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for
details.
