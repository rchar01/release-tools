# Release Procedures

This guide describes common release scenarios for repositories that consume the
installed `release-tools` CLI. It is procedure-oriented; the canonical variable
contract remains [`usage.md`](usage.md).

The core split is intentional:

- `release-tools` owns release orchestration, release notes, Helm chart
  packaging, chart publishing, chart metadata, and clean-tag publishing
- GoReleaser owns binaries, archives, checksums, container images, container
  manifests, and GoReleaser-supported signing configured in `.goreleaser.yaml`
- Helm owns chart validation, package creation, OCI chart pushes, and optional
  classic provenance files

Do not add consumer-facing Make targets or copied release wrapper scripts. A
consumer repository should run `release-tools` directly from `PATH`.

## Choose A Scenario

Use the smallest release shape that matches the repository:

| Scenario | Release Tools Config | GoReleaser Config |
| --- | --- | --- |
| Binary-only release | default or `RELEASE_ARTIFACTS=binaries` | `builds`, `archives`, `checksum`, forge release config |
| Binary and container image release | default or `RELEASE_ARTIFACTS=binaries` | binary config plus `dockers`, `dockers_v2`, `docker_manifests`, or `docker_signs` |
| Binary and Helm chart release | `RELEASE_ARTIFACTS=binaries,charts` plus Helm variables | normal binary config |
| Binary, container image, and Helm chart release | `RELEASE_ARTIFACTS=binaries,charts` plus Helm variables | binary config plus container image config |

Container images are not a `RELEASE_ARTIFACTS` value. Keep container image
configuration in `.goreleaser.yaml`.

## Required Files

A consumer repository usually starts with these files:

- `.release-tools.env`
- `.goreleaser.yaml`
- `NEWS.md` when release notes come from a changelog file
- `charts/<name>/Chart.yaml` when publishing Helm charts
- `Containerfile` or another GoReleaser-referenced image build file when
  publishing container images
- a CI workflow when releases are published from CI

The chart config example is [`../examples/chart-release.env`](../examples/chart-release.env).

## Binary-Only Release

Use this for a normal Go CLI or toolkit release where GoReleaser creates the
release assets.

Minimal `.release-tools.env`:

```sh
RELEASE_PROJECT=myapp
RELEASE_FORGE=codeberg
RELEASE_OWNER=myowner
RELEASE_REPO=myapp
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml
```

The project-owned `.goreleaser.yaml` should define the build matrix, archive
names, checksum file, and forge release settings. `release-tools` invokes
GoReleaser but does not redefine those details.

## Binary And Container Image Release

Use this when the release should publish binaries and container images.

Keep `.release-tools.env` the same as the binary-only case unless charts are
also enabled. Configure images in `.goreleaser.yaml`.

Minimal Podman-backed GoReleaser example:

```yaml
version: 2

project_name: myapp

builds:
  - id: myapp
    main: ./cmd/myapp
    binary: myapp
    env:
      - CGO_ENABLED=0
    goos:
      - linux
    goarch:
      - amd64

dockers:
  - image_templates:
      - registry.example.com/myowner/myapp:{{ .Tag }}
      - registry.example.com/myowner/myapp:latest
    dockerfile: Containerfile
    use: podman
    build_flag_templates:
      - --pull
      - --label=org.opencontainers.image.version={{ .Version }}

checksum:
  name_template: checksums.txt
```

Run `release-tools doctor` before publishing. When it sees supported top-level
GoReleaser container keys, it checks for the local tools those keys imply. It
does not replace GoReleaser's image build, push, manifest, or signing behavior.

## Binary And Helm Chart Release

Use this when the release should package one or more Helm charts and optionally
publish them after GoReleaser succeeds.

Minimal `.release-tools.env`:

```sh
RELEASE_PROJECT=myapp
RELEASE_FORGE=codeberg
RELEASE_OWNER=myowner
RELEASE_REPO=myapp
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml

RELEASE_ARTIFACTS=binaries,charts
RELEASE_HELM_CHART_DIRS=charts/myapp
RELEASE_HELM_VERSION_FROM=tag
RELEASE_HELM_APP_VERSION_FROM=tag
```

Add OCI chart publishing when the target registry is Helm OCI-compatible:

```sh
RELEASE_HELM_OCI_REPOSITORY=oci://registry.example.com/myowner/charts
RELEASE_HELM_OCI_USERNAME=robot
RELEASE_HELM_OCI_PASSWORD_FILE=~/.config/helm/oci-token
```

Add classic ChartMuseum-compatible publishing when the target registry expects a
classic package upload, including Forgejo or Gitea package registries:

```sh
RELEASE_HELM_CLASSIC_URL=https://forge.example/api/packages/myowner/helm
RELEASE_HELM_CLASSIC_USERNAME=robot
RELEASE_HELM_CLASSIC_TOKEN_FILE=~/.config/forgejo/helm-token
```

Add classic Helm provenance files only when the project has a Helm-compatible GPG
signing keyring outside the repository:

```sh
RELEASE_HELM_PROVENANCE=1
RELEASE_HELM_GPG_KEY=maintainer@example.org
RELEASE_HELM_GPG_KEYRING=~/.config/helm/release-keyring.gpg
```

Add OCI chart signing only after OCI chart pushes work reliably:

```sh
RELEASE_HELM_OCI_SIGNER=cosign
RELEASE_HELM_OCI_SIGN_ARGS=--key cosign.key
```

Chart directories must be repo-relative, stay inside the repository, and avoid
path components beginning with `-`.

## Binary, Container Image, And Helm Chart Release

Use this when the release should publish all artifact types.

Start with the chart-enabled `.release-tools.env` from the previous section and
add the container image configuration to `.goreleaser.yaml`. The command flow is
unchanged:

```bash
release-tools doctor
release-tools check
VERSION=v1.2.3 release-tools snapshot
release-tools publish-tag v1.2.3
```

Expected ordering during `publish` and `publish-tag`:

- `release-tools` packages charts before GoReleaser publish starts
- GoReleaser publishes binaries, release assets, and configured container images
- `release-tools` pushes or uploads packaged charts after GoReleaser succeeds
- optional chart signing and release-manifest generation happen after chart
  publishing succeeds

This ordering means a chart registry failure can happen after the forge release
and container image publish have already succeeded. Fix the registry issue and
rerun the publish command only after confirming duplicate release assets or chart
versions will not be overwritten unintentionally.

## Staging Rollout

Use a disposable or staging target before first production use.

1. Add the minimum `.release-tools.env` and `.goreleaser.yaml` changes.
2. Run `release-tools doctor` and fix missing tools or invalid config.
3. Run `release-tools check` and fix GoReleaser or Helm validation failures.
4. Run `VERSION=v0.0.0-test release-tools snapshot` and inspect `dist/`.
5. Configure a staging forge repository or disposable release target.
6. Configure a staging container registry if container images are enabled.
7. Configure a staging Helm OCI or classic registry if charts are enabled.
8. Create and push the staging tag, for example
   `git tag -a v0.0.0-test -m "v0.0.0-test"` followed by
   `git push <remote> v0.0.0-test`.
9. Publish the existing staging tag with
   `release-tools publish-tag v0.0.0-test`.
10. Verify forge assets, container images, chart packages, and
   `dist/release-manifest.json`.
11. Move the same config pattern to the production repository or registry.

Do not use a production signing key or production registry token for the first
end-to-end staging run.

## Credentials Checklist

Keep tokens out of committed `.release-tools.env` files.

| Purpose | Where To Configure |
| --- | --- |
| Forge release publishing | `RELEASE_TOKEN`, native forge token variable, or environment-only `RELEASE_TOKEN_FILE` |
| Container registry publishing | Docker, Podman, registry helper, or GoReleaser-supported environment |
| Helm OCI publishing | prior Helm login, or `RELEASE_HELM_OCI_USERNAME` plus `RELEASE_HELM_OCI_PASSWORD_FILE` or environment-only `RELEASE_HELM_OCI_PASSWORD` |
| Classic Helm package upload | `RELEASE_HELM_CLASSIC_USERNAME` plus `RELEASE_HELM_CLASSIC_TOKEN_FILE` or environment-only `RELEASE_HELM_CLASSIC_TOKEN` |
| GoReleaser binary or image signing | GoReleaser config and signer-owned environment |
| Helm classic provenance signing | `RELEASE_HELM_GPG_KEY` and `RELEASE_HELM_GPG_KEYRING` |
| Helm OCI chart signing | `RELEASE_HELM_OCI_SIGNER=cosign` and Cosign-owned credentials |

`RELEASE_TOKEN_FILE`, `RELEASE_HELM_OCI_PASSWORD`, and
`RELEASE_HELM_CLASSIC_TOKEN` are environment-only values. Do not put them in
repo-local config.

## Preflight Commands

Run these from the repository root:

```bash
release-tools version
release-tools doctor
release-tools check
VERSION=v1.2.3 release-tools snapshot
```

Use a real version-like value for snapshot testing, either from `VERSION` or an
exact current tag, because chart versions are derived from the release tag value.
The current chart version source is `tag`.

## Publish Commands

Publish an existing tag from a clean clone:

```bash
export RELEASE_TOKEN_FILE=~/.config/forge/token
release-tools publish-tag v1.2.3
```

If the current commit is already the exact release tag, `publish` is also
available:

```bash
export RELEASE_TOKEN_FILE=~/.config/forge/token
release-tools publish
```

Prefer `publish-tag` for normal releases because it publishes from a clean
temporary clone of the exact tag and reloads repo-local release config from that
clone.

## Failure And Retry Notes

Check failures are local and should be fixed before tagging.

Snapshot failures do not publish remote artifacts. Remove or inspect `dist/`, fix
the issue, and rerun the snapshot.

GoReleaser publish failures can leave partial forge or registry state depending
on where the failure occurred. Inspect the forge release, published assets,
container tags, and registry logs before retrying.

Chart publish failures can occur after GoReleaser succeeds. Inspect chart registry
state and `dist/release-manifest.json`; the manifest should only claim completed
chart publish steps.

Duplicate release assets, duplicate chart versions, or duplicate manifest uploads
fail instead of silently replacing existing outputs. Decide whether to delete the
bad remote object manually, choose a new tag, or leave the failed release in place
before retrying.
