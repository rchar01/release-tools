# AGENTS.md

## Scope
- This repo is a shared release toolkit for Go, shell, and documentation/toolkit projects.
- Consumer repos are expected to install `release-tools` into `PATH` and keep only release config locally.
- Keep repo-specific release config in the consumer repo; keep shared release behavior in the CLI.
- The installed CLI is the only public command surface for v2+.
- The root `Makefile` is maintainer-only convenience for this repo; do not document Make as a consumer release frontend.

## Agent Workflow Expectations
- Read relevant code before editing
- Prefer minimal changes that match existing patterns
- Use a verification-focused subagent for non-trivial test runs or runtime-backed checks when available
- Use a review-focused subagent after substantial edits to catch regressions and doc/code drift when available
- Use a research-focused subagent when behavior depends on external tooling or upstream docs when available
- Summarize any subagent findings you rely on
- Do not revert unrelated worktree changes

## Read First
- `README.md`
- `docs/README.md`
- `docs/usage.md`
- `docs/agent-release-flow.md`
- `cmd/release-tools/main.go`
- `.release-tools.env`
- `.goreleaser.yaml`
- `Containerfile.dev`
- `Makefile`
- `scripts/test`
- `scripts/test-errors`

## Repo Shape
- `cmd/release-tools/`: Go CLI source of truth for public release behavior
- `Makefile`: maintainer-only local targets for this repo
- `.release-tools.env`: self-release config for this repo
- `.goreleaser.yaml`: self-release artifact config
- `examples/`: ready-to-copy consumer integration files
- `docs/README.md`: short docs index
- `docs/usage.md`: public integration contract and end-to-end consumer setup guide
- `docs/agent-release-flow.md`: rationale and invariants for the release flow
- `scripts/`: development and verification helpers

## Public Contract
- `docs/usage.md` is the canonical public variable contract. Update it when
  adding, removing, or changing supported caller-provided variables.
- Keep the split between supported `.release-tools.env` keys and
  environment-only variables documented in `docs/usage.md`.
- Environment-only variables documented in `docs/usage.md` must not be accepted
  from repo-local config unless the public contract intentionally changes.
- High-risk invariants to preserve unless intentionally changing the public
  contract: installed CLI only, `.release-tools.env` default config,
  environment overrides, publishing-only token mapping, no token passthrough for
  validation commands, GoReleaser-owned binaries/containers, Helm-owned chart
  packaging, and no consumer-facing Make release frontend.

## Commands
- CLI:
  - `release-tools tools-check`
  - `release-tools version`
  - `release-tools doctor`
  - `release-tools check`
  - `release-tools snapshot`
  - `release-tools publish`
  - `release-tools publish-tag vX.Y.Z`
  - `release-tools notes vX.Y.Z`
  - `release-tools completion bash|zsh|fish|powershell`
- Verification:
  - `make verify`
  - `make container-test`
  - `make helm-registry-test` for Podman-backed Helm registry smoke tests
  - `make helm-oci-signing-test` for Podman-backed Helm OCI Cosign smoke tests
    with trusted `cosign` available on `PATH`
  - `make helm-provenance-test` for disposable GPG-backed Helm provenance smoke
    tests
  - `make codeberg-smoke-test` for live Codeberg release, manifest asset, and
    optional Helm package smoke tests against the disposable
    `rch/release-tools-smoke` repository
  - `scripts/test-errors` for focused error-message checks

## Self-Release Procedure
- Do not use Make as the publish frontend.
- Follow `docs/self-release.md` as the canonical maintainer self-release
  procedure.
- Use `make verify` and `make container-test` for release verification before
  tagging.
- Use `make helm-registry-test` before releases that change Helm registry
  publishing behavior; fixture-based Helm smoke tests must run `release-tools`
  with a sanitized environment so live release credentials cannot override the
  fixture config.
- Use `make helm-oci-signing-test` before releases that change Helm OCI signing
  behavior.
- Use `make helm-provenance-test` before releases that change Helm provenance
  signing behavior.
- Use `make codeberg-smoke-test` only with a token that can push to the smoke
  repository and create releases; package-registry access is needed to exercise
  the Helm upload portion. Live smoke-test tokens must stay outside the
  dev-container build context and be mounted only at container run time; the
  host `realpath` tool is required for token-path validation.
- Add the release entry to `NEWS.md` and move `CHANGELOG.md` `Unreleased`
  entries to the release version before committing release prep.
- Build the current CLI with `make build` before publishing this repository.
- Publish with `./.tmp/release-tools publish-tag vX.Y.Z`.
- This intentionally invokes the just-built CLI by path so self-release does not
  depend on an older globally installed binary, without trusting every executable
  in the repo-local `.tmp` directory during a privileged publish.
- Ensure `RELEASE_TOKEN`, the native forge token variable, or
  environment-only `RELEASE_TOKEN_FILE` is available before publishing.

## Verified Behavior To Preserve
- Keep the installed `release-tools` binary as the only public command surface.
- Keep Make targets maintainer-only; consumer repos should call `release-tools` from `PATH`.
- The CLI fails fast on missing `RELEASE_PROJECT` and `RELEASE_OWNER`; tag publishing also requires `VERSION` or a positional tag.
- `release-tools check` runs `goreleaser check`; when charts are enabled it also
  runs `helm dependency update --skip-refresh` and `helm lint` for each chart.
- `release-tools snapshot` runs `goreleaser release --snapshot --skip=publish
  --clean`; when charts are enabled it also runs `helm package` into
  `dist/charts`.
- Helm chart directory config rejects path components beginning with `-`, and
  Helm commands pass chart paths after `--` so chart paths cannot be interpreted
  as Helm options.
- `publish` and `publish-tag` package charts before GoReleaser publish starts;
  when `RELEASE_HELM_OCI_REPOSITORY` is set they push packaged charts with
  `helm push` after GoReleaser succeeds.
- Publish-time chart packages are written to a temporary directory outside the
  release repository so GoReleaser `--clean` cannot delete them before upload.
- Explicit Helm OCI auth is validated before GoReleaser publish starts, but
  `helm registry login --password-stdin --registry-config <temporary-file>` runs
  only after GoReleaser succeeds and immediately before `helm push`.
- When `RELEASE_HELM_CLASSIC_URL` is set, `publish` and `publish-tag` upload
  packaged charts to `<url>/api/charts` after GoReleaser succeeds.
- Chart-enabled snapshot, publish, and publish-tag flows write
  `dist/release-manifest.json` with the release tag, chart version, packaged
  chart path, SHA-256, optional provenance file path/SHA-256, and configured
  Helm registry targets after packaging or chart upload succeeds.
- When GoReleaser writes `dist/artifacts.json`, snapshot, publish, and
  publish-tag merge GoReleaser artifact metadata into
  `dist/release-manifest.json` without changing GoReleaser artifact ownership.
- `publish-tag` publishes from a clean temporary clone of the exact tag.
- `publish-tag` reloads repo-local release config from the clean temporary clone
  and must not carry dirty/current-worktree `.release-tools.env` values into
  clone execution; operator-provided environment values, including release
  tokens, remain available.
- `publish-tag` copies chart outputs, GoReleaser artifact files referenced by
  `dist/release-manifest.json`, and the manifest back from the temporary clone
  so repo-relative manifest paths remain valid locally; GoReleaser artifact
  and chart copy-back paths must stay under `dist/` and must be regular files,
  and manifest read/write/remove paths must not follow symlinked parents or
  targets.
- GoReleaser must run from the release repository root.
- unset `RELEASE_ARTIFACTS` keeps current binaries-only behavior.
- `check` and `snapshot` paths must not require `RELEASE_TOKEN` and must not
  pass release-token variables through to GoReleaser.
- CLI release notes currently support `RELEASE_NOTES_MODE=news-md`, `gnu-news`,
  and `none`.
- CLI release body patching currently supports `RELEASE_BODY_MODE=patch` and `none`.
- `RELEASE_MANIFEST_UPLOAD=1` uploads `dist/release-manifest.json` as a forge
  release asset after all configured publish-time artifact steps and local
  manifest generation succeed; duplicate upload/link responses fail rather than
  replace existing assets.
- project Go preflight is required only when `RELEASE_REQUIRE_GO=1`.
- GoReleaser container image preflights require Docker, Podman, Cosign, or a
  configured static signing command only when the GoReleaser config contains
  matching top-level container keys.
- `VERSION` is the only supported tag override variable; `TAG` is not public config.

## Tooling / Env Notes
- the CLI requires a resolvable `goreleaser`.
- release body patching uses the Go HTTP client.
- publishing token resolution reads `RELEASE_TOKEN`, the native GoReleaser token
  variable for `RELEASE_FORGE`, or environment-only `RELEASE_TOKEN_FILE` in that
  order.
- GoReleaser resolution checks `GORELEASER_BIN`, then common install locations.
- Helm is required only when `RELEASE_ARTIFACTS` includes `charts`.
- Helm chart provenance signing uses Helm's built-in `helm package --sign` with
  an explicit GPG key and keyring; OCI chart signing uses Cosign against the
  immutable digest reported by Helm.
- Docker/Podman are required only when GoReleaser container image config is
  detected; Cosign is required when GoReleaser image signing config is detected
  or `RELEASE_HELM_OCI_SIGNER=cosign` is configured.
- Go baseline is Go 1.26 with toolchain `go1.26.4`.
- Dev-container verification uses Podman through `scripts/in-container`; the dev
  container is the source of required development tools, including Helm and
  Cosign, and uses `--userns=keep-id` to keep mounted workspace writes
  host-user-writable under rootless Podman.
- Dev-container downloaded release-tool archives are verified with pinned
  SHA-256 checksums before installation.

## Editing Notes
- When changing documented behavior, update the matching docs in `docs/usage.md` and `docs/agent-release-flow.md`.
- Prefer executable sources over prose if they conflict.
- Do not add consumer-repo assumptions that are not enforced by this toolkit.
- Do not add Make as a consumer release frontend.
