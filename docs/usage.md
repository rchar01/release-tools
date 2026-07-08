# Release Tools Usage

This document describes the `release-tools` v3 installed-CLI model.

The goal is:

- keep shared release behavior in `release-tools`
- keep project-specific release config in the consuming repo
- install one `release-tools` binary into `PATH`
- run `release-tools` directly from the project root
- avoid copied bootstrap or release helper scripts in consumer repos

Ready-to-copy starting points are included in this repository:

- `examples/.release-tools.env`
- `examples/chart-release.env`
- `examples/forgejo-release.yml`

Use `examples/chart-release.env` by copying it into the consuming repository as
`.release-tools.env`, or set `RELEASE_CONFIG_FILE` explicitly when using another
file name.

## Required Project Files

A consuming project should have at least these files:

- `.release-tools.env`
- `.goreleaser.yaml`
- `NEWS.md` if release notes are generated from changelog entries
- CI release workflow if publishing from CI

Depending on the project, it may also have project-owned build or installer
scripts, but `release-tools` does not require any copied helper script.

## 1. Install The CLI

Download the matching release binary and place it in a directory on `PATH`.

Linux amd64 example:

```bash
mkdir -p "$HOME/.local/bin"
curl -fsSL -o "$HOME/.local/bin/release-tools" \
  "https://codeberg.org/rch/release-tools/releases/download/v3.3.0/release-tools_3.3.0_linux_amd64"
chmod +x "$HOME/.local/bin/release-tools"
```

Published binary names follow this shape:

```text
release-tools_<version>_<os>_<arch>
```

Supported release binaries:

- `release-tools_3.3.0_linux_amd64`
- `release-tools_3.3.0_linux_arm64`
- `release-tools_3.3.0_darwin_amd64`
- `release-tools_3.3.0_darwin_arm64`

For system-wide installation, use a privileged install directory instead:

```bash
sudo install -m 0755 release-tools_3.3.0_linux_amd64 /usr/local/bin/release-tools
```

## 2. Define Release Tools Config

Add `.release-tools.env` at the repository root.

Example:

```sh
RELEASE_PROJECT=platformctl
RELEASE_FORGE=codeberg
RELEASE_OWNER=rch
RELEASE_REPO=platformctl
# RELEASE_TOKEN_FILE=~/.config/forge/token
RELEASE_NOTES_SOURCE=NEWS.md
RELEASE_NOTES_MODE=news-md
RELEASE_BODY_MODE=patch
GORELEASER_CONFIG=.goreleaser.yaml
```

Supported `RELEASE_FORGE` values:

- `codeberg`
- `gitea`
- `forgejo`
- `github`
- `gitlab`

`codeberg`, `gitea`, and `forgejo` use Codeberg-compatible defaults unless
`RELEASE_API_URL` is set explicitly.

## 3. Review Supported Variables

Supported `.release-tools.env` keys:

- `RELEASE_PROJECT`
- `RELEASE_FORGE`
- `RELEASE_OWNER`
- `RELEASE_REPO`
- `RELEASE_API_URL`
- `RELEASE_ARTIFACTS`
- `RELEASE_HELM_CHART_DIRS`
- `RELEASE_HELM_VERSION_FROM`
- `RELEASE_HELM_APP_VERSION_FROM`
- `RELEASE_HELM_OCI_REPOSITORY`
- `RELEASE_HELM_OCI_USERNAME`
- `RELEASE_HELM_OCI_PASSWORD_FILE`
- `RELEASE_HELM_OCI_PLAIN_HTTP`
- `RELEASE_HELM_CLASSIC_URL`
- `RELEASE_HELM_CLASSIC_USERNAME`
- `RELEASE_HELM_CLASSIC_TOKEN_FILE`
- `RELEASE_HELM_PROVENANCE`
- `RELEASE_HELM_GPG_KEY`
- `RELEASE_HELM_GPG_KEYRING`
- `RELEASE_NOTES_SOURCE`
- `RELEASE_NOTES_MODE`
- `RELEASE_BODY_MODE`
- `GORELEASER_CONFIG`
- `GORELEASER_BIN`
- `RELEASE_REQUIRE_GO`
- `RELEASE_TOKEN_FILE`

Supported environment-only variables:

- `RELEASE_CONFIG_FILE`
- `RELEASE_TOKEN`
- native GoReleaser token variables: `GITEA_TOKEN`, `GITHUB_TOKEN`, or
  `GITLAB_TOKEN`
- `RELEASE_HELM_OCI_PASSWORD`
- `RELEASE_HELM_CLASSIC_TOKEN`
- `VERSION`

Environment variables override `.release-tools.env` values. Set
`RELEASE_CONFIG_FILE` to load a different config file.

Artifact classes are configured with `RELEASE_ARTIFACTS`:

```sh
RELEASE_ARTIFACTS=binaries
RELEASE_ARTIFACTS=binaries,charts
```

If unset, `release-tools` keeps the existing binaries-only behavior. Supported
values are `binaries` and `charts`. The `doctor` command validates and reports
the configured artifact classes.

When charts are enabled, list one or more chart directories relative to the
repository root:

```sh
RELEASE_ARTIFACTS=binaries,charts
RELEASE_HELM_CHART_DIRS=charts/myapp
RELEASE_HELM_VERSION_FROM=tag
RELEASE_HELM_APP_VERSION_FROM=tag
# RELEASE_HELM_OCI_REPOSITORY=oci://registry.example.com/myowner/charts
# RELEASE_HELM_OCI_USERNAME=robot
# RELEASE_HELM_OCI_PASSWORD_FILE=~/.config/helm/oci-token
# RELEASE_HELM_OCI_PLAIN_HTTP=0
# RELEASE_HELM_CLASSIC_URL=https://forge.example/api/packages/myowner/helm
# RELEASE_HELM_CLASSIC_USERNAME=robot
# RELEASE_HELM_CLASSIC_TOKEN_FILE=~/.config/forgejo/helm-token
# RELEASE_HELM_PROVENANCE=0
# RELEASE_HELM_GPG_KEY=maintainer@example.org
# RELEASE_HELM_GPG_KEYRING=~/.gnupg/secring.gpg
```

Only `tag` is currently supported for Helm chart and app versions. A release tag
such as `v1.2.3` becomes chart version `1.2.3`.

Chart-enabled commands add local Helm behavior:

- `release-tools doctor` validates `helm`, chart directories, and `Chart.yaml`
- `release-tools check` runs `helm dependency update --skip-refresh` and
  `helm lint` for each chart after `goreleaser check`
- `release-tools snapshot` runs the GoReleaser snapshot and packages charts into
  `dist/charts`
- `release-tools publish` and `release-tools publish-tag` package charts before
  GoReleaser publishes release assets
- when `RELEASE_HELM_OCI_REPOSITORY` is set, `publish` and `publish-tag` run
  `helm push <chart>.tgz oci://...` for each packaged chart after GoReleaser
  succeeds
- when `RELEASE_HELM_CLASSIC_URL` is set, `publish` and `publish-tag` upload
  each packaged chart to `<url>/api/charts` after GoReleaser succeeds
- when `RELEASE_HELM_PROVENANCE=1`, chart packaging adds Helm provenance files
  beside the packaged charts
- chart-enabled snapshot, publish, and publish-tag flows write
  `dist/release-manifest.json` with chart package metadata

Snapshot chart packaging needs `VERSION` or an exact current tag so the chart
version can be derived from the release tag.

`RELEASE_HELM_OCI_REPOSITORY` must be an `oci://` repository path such as
`oci://registry.example.com/myowner/charts`. Do not include the chart name or
version tag; Helm derives those from the packaged chart. The caller must run
`helm registry login` or otherwise provide Helm registry credentials before
publishing to a private registry. If `RELEASE_HELM_OCI_USERNAME` is set with
`RELEASE_HELM_OCI_PASSWORD_FILE` or environment-only `RELEASE_HELM_OCI_PASSWORD`,
`release-tools` runs `helm registry login` with `--password-stdin` and
`--registry-config <temporary-file>` before pushing charts, then uses that
temporary registry config for `helm push`. Plaintext
`RELEASE_HELM_OCI_PASSWORD` is intentionally not accepted in `.release-tools.env`.
Set `RELEASE_HELM_OCI_PLAIN_HTTP=1` only for disposable or otherwise explicitly
trusted insecure registries; it appends Helm's `--plain-http` flag to OCI chart
registry login and pushes.

Set `RELEASE_HELM_PROVENANCE=1` to sign packaged charts with Helm's classic
provenance support. When enabled, `RELEASE_HELM_GPG_KEY` and
`RELEASE_HELM_GPG_KEYRING` are required, and `release-tools` runs
`helm package ... --sign --key <key> --keyring <keyring>`. Relative keyring
paths are resolved from the release repository root, including the clean tag
clone used by `publish-tag`. The keyring must be readable before publish starts.
OCI chart signing is not implemented; chart OCI signatures are intentionally
deferred until digest-based signing behavior is validated against real
registries.

`RELEASE_HELM_CLASSIC_URL` is for Forgejo/Gitea-compatible classic Helm package
registries. Set it to the Helm package base URL, for example
`https://forge.example/api/packages/myowner/helm`. Do not include credentials,
query strings, fragments, or the `/api/charts` upload suffix in this URL.
`release-tools` uploads each packaged chart with a raw `POST` to
`<url>/api/charts` using Basic auth. Set
`RELEASE_HELM_CLASSIC_USERNAME` and provide the package token or password with
`RELEASE_HELM_CLASSIC_TOKEN_FILE` or environment-only
`RELEASE_HELM_CLASSIC_TOKEN`. Plaintext `RELEASE_HELM_CLASSIC_TOKEN` is
intentionally not accepted in `.release-tools.env`. If the package registry
rejects a duplicate chart version, `release-tools` fails the upload rather than
overwriting it.

The chart release manifest currently uses this schema:

```json
{
  "schema_version": 1,
  "release": {
    "tag": "v1.2.3",
    "version": "1.2.3"
  },
  "artifacts": {
    "helm_charts": [
      {
        "name": "myapp",
        "version": "1.2.3",
        "path": "dist/charts/myapp-1.2.3.tgz",
        "sha256": "...",
        "provenance_path": "dist/charts/myapp-1.2.3.tgz.prov",
        "provenance_sha256": "...",
        "oci_ref": "oci://registry.example.com/myowner/charts/myapp:1.2.3",
        "classic_url": "https://forge.example/api/packages/myowner/helm",
        "classic_upload_url": "https://forge.example/api/packages/myowner/helm/api/charts"
      }
    ]
  }
}
```

The provenance, OCI, and classic fields appear only when those outputs or
targets are configured. Manifest chart package paths are repo-relative
`dist/charts/...` paths. During publish commands, charts are first packaged in a
temporary directory outside the repository so GoReleaser cannot clean them before
upload. After chart pushes or uploads succeed, those packages and `.prov` files
are copied back into `dist/charts` before the manifest is written. For
`publish-tag`, the chart packages, provenance files, and manifest are copied
from the clean temporary tag clone back to the caller repository. The manifest
is not yet uploaded as a release asset and does not yet merge GoReleaser's
`dist/artifacts.json` metadata.

Required for release commands:

- `RELEASE_PROJECT`
- `RELEASE_OWNER`

Additionally required for `release-tools publish-tag`:

- `VERSION` or a positional tag argument such as `v3.3.0`

## 4. Add GoReleaser Configuration

The consuming repo owns `.goreleaser.yaml`.

`release-tools` does not define:

- build matrix
- archive names
- binary names
- checksum names
- before hooks
- container image names, Dockerfiles, buildx settings, manifest templates, or
  image-signing policy

The project should define those directly.

The config should be compatible with these CLI commands:

- `release-tools version`
- `release-tools doctor`
- `release-tools check`
- `release-tools snapshot`
- `release-tools publish`
- `release-tools publish-tag vX.Y.Z`
- `release-tools notes vX.Y.Z`
- `release-tools completion bash|zsh|fish|powershell`

Recommended for Go projects:

- use repo-local `.tmp/go-build` and `.tmp/go-cache` in hooks
- set `RELEASE_REQUIRE_GO=1` if the release flow requires a local Go toolchain
- keep archives to the exact contract expected by downstream consumers
- keep binary names stable across builds and installs

Shell or documentation toolkits can use GoReleaser meta archives or source
archives without configuring a project Go build. `release-tools` itself ships as
a compiled Go CLI. Consuming projects only need Go when their own release flow
requires it.

Container image publishing remains configured in `.goreleaser.yaml`. When
`doctor` or `tools-check` sees top-level `dockers`, `dockers_v2`,
`docker_manifests`, or `docker_signs`, it performs only local tool preflights:

- `dockers` requires `docker` by default, or `podman` when an entry sets
  `use: podman`
- `dockers_v2` requires `docker` because GoReleaser uses Docker buildx
- `docker_manifests` requires `docker` by default, or `podman` when an entry
  sets `use: podman`
- `docker_signs` requires `cosign` by default, or the static command named by an
  entry's `cmd` value

These checks do not make container images a `RELEASE_ARTIFACTS` value and do not
replace GoReleaser's image build, push, manifest, or signing behavior.
Dynamic or templated signing command values cannot be resolved by
`release-tools`; block-scalar `cmd` values are also left for GoReleaser to
validate at release time.

## 5. Add A Release Notes Source

If the project generates release notes from `NEWS.md`, choose one of the
supported modes:

- `RELEASE_NOTES_MODE=news-md`
- `RELEASE_NOTES_MODE=gnu-news`

Use `news-md` for Markdown sections matching release tags.

Example shape:

```md
## v2.0.0 - 2026-06-11

- switch to the CLI-only release workflow
- add self-release support

## v1.2.1 - 2026-06-01

- clarify the release-tools value proposition
```

Generated release notes are release body text only. The toolkit does not add a
top-level Markdown heading for the tag because the forge already renders the
release title separately.

Use `gnu-news` for the GNU-style subset used by this toolkit:

```text
release-tools NEWS -- history of user-visible changes.

* Noteworthy changes in release 2.0.0 (2026-06-11)

** New features

  - Switch to the CLI-only release workflow.
  - Add self-release support.

* Noteworthy changes in release 1.2.1 (2026-06-01)

** Documentation

  - Clarify the release-tools value proposition.
```

The `gnu-news` parser matches either `X.Y.Z` or `vX.Y.Z` in the release
heading, stops at the next GNU release heading, and preserves the body below the
heading in the final release notes file. That includes `**` subsection headings
and indented bullets.

## 6. Add CI Release Workflow

CI should:

- checkout full history
- install GoReleaser
- install a pinned `release-tools` binary
- run project tests
- run `release-tools publish` from the project root

Use `examples/forgejo-release.yml` in this repository as a ready-to-copy
starting point.

## 7. Token And Auth Contract

The public token name is:

- `RELEASE_TOKEN`

Local maintainer flow should support:

- `RELEASE_TOKEN` from the environment
- or the native GoReleaser token variable for `RELEASE_FORGE`
- or `RELEASE_TOKEN_FILE` pointing at a local token file

Recommended local setup:

```sh
RELEASE_TOKEN_FILE=~/.config/forge/token
```

CI should provide `RELEASE_TOKEN` through repository secrets.

Token resolution order is:

1. `RELEASE_TOKEN`
2. native forge token variable, such as `GITEA_TOKEN`
3. `RELEASE_TOKEN_FILE`

`RELEASE_TOKEN_FILE` supports `~`, `$HOME`, and `${HOME}` at the start of the
path. The file content is read only for `publish` and `publish-tag`; trailing
newlines are trimmed and the token is not printed.

`release-tools` maps that internally to the token variable GoReleaser expects:

- `RELEASE_FORGE=codeberg`, `gitea`, or `forgejo`: `GITEA_TOKEN`
- `RELEASE_FORGE=github`: `GITHUB_TOKEN`
- `RELEASE_FORGE=gitlab`: `GITLAB_TOKEN`

## 8. Runtime Contract And Command Surface

Run commands from the consumer repo root:

```bash
release-tools version
release-tools doctor
release-tools check
release-tools snapshot
release-tools publish
release-tools publish-tag v3.3.0
release-tools notes v3.3.0
```

The CLI also generates shell completion scripts:

```bash
release-tools completion bash
release-tools completion zsh
release-tools completion fish
release-tools completion powershell
```

Install or source the generated script according to your shell's completion
setup. Completion generation, `help`, and `version` do not require a release
config file.

`release-tools publish-tag` publishes from a clean full-history clone detached at
the exact repo tag. Keeping tag history available lets GoReleaser discover the
previous tag for changelog generation.

The CLI fails fast when required variables are missing:

- `release-tools doctor`
- `release-tools check`
- `release-tools snapshot`
- `release-tools publish`
- `release-tools notes`
  require `RELEASE_PROJECT` and `RELEASE_OWNER`
- `release-tools publish-tag`
  requires `RELEASE_PROJECT`, `RELEASE_OWNER`, and `VERSION` or a positional tag

## 9. What Should Stay In The Consumer Repo

These stay local to the project:

- `.goreleaser.yaml`
- `.release-tools.env`
- `NEWS.md`
- project build/install scripts
- CI workflow details
- artifact naming contract

## 10. What Should Stay In release-tools

These stay shared in `release-tools`:

- CLI command dispatch
- tool checks
- config loading
- token resolution helpers
- GoReleaser invocation wrapper
- check/snapshot/publish logic
- safe `publish-tag` clean-clone logic
- release-notes generation logic
- release body patch logic

The consuming project should not duplicate these behaviors.

## 11. Minimum Verification Checklist

After wiring a project to `release-tools`, verify:

1. `release-tools version`
2. `release-tools doctor`
3. `release-tools check`
4. `release-tools snapshot`
5. `release-tools notes <current-tag>`
6. `release-tools publish-tag <older-project-tag>`

For the last command, it is fine to test with an intentionally invalid token if
you only want to verify the flow gets through clone, notes, and build without
actually publishing.

Older project tags can still fail if the tagged source tree does not contain the
project files expected by the current toolkit, such as `.goreleaser.yaml` or
`NEWS.md` when `RELEASE_NOTES_MODE=news-md` or `gnu-news` is used.

## 12. Recommended Consumer Template

For a new project, the minimum integration shape is:

```text
.
├── .goreleaser.yaml
├── .release-tools.env
├── NEWS.md
└── <ci-release-workflow>
```

That is the configuration `release-tools` assumes and documents for installed
CLI consumers.
