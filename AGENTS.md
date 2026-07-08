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
- Supported caller-provided variables are:
  - `RELEASE_CONFIG_FILE`
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
  - `RELEASE_NOTES_SOURCE`
  - `RELEASE_NOTES_MODE`
  - `RELEASE_BODY_MODE`
  - `GORELEASER_CONFIG`
  - `GORELEASER_BIN`
  - `RELEASE_REQUIRE_GO`
  - `RELEASE_TOKEN_FILE`
  - `RELEASE_TOKEN`
  - `VERSION`
- `.release-tools.env` is the default repo-local config file.
- Environment variables override `.release-tools.env` values.
- `RELEASE_TOKEN` is the public forge-token variable.
- `RELEASE_TOKEN_FILE` may point at a local file containing the forge token.
- The CLI maps `RELEASE_TOKEN` to `GITEA_TOKEN`, `GITHUB_TOKEN`, or
  `GITLAB_TOKEN` internally for GoReleaser based on `RELEASE_FORGE`.
- Supported `RELEASE_FORGE` values are `codeberg`, `gitea`, `forgejo`,
  `github`, and `gitlab`.
- `RELEASE_ARTIFACTS` defaults to `binaries`; supported values are `binaries`
  and `charts`.
- `RELEASE_HELM_CHART_DIRS` is required when `RELEASE_ARTIFACTS` includes
  `charts`.
- Supported Helm version source values are currently `tag` only.
- `RELEASE_HELM_OCI_REPOSITORY` enables `helm push` to an OCI repository during
  `publish` and `publish-tag`; Helm registry authentication is caller-owned.

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
  - `scripts/test-errors` for focused error-message checks

## Self-Release Procedure
- Do not use Make as the publish frontend.
- Use `make verify` and `make container-test` for release verification before
  tagging.
- Update `NEWS.md` and `CHANGELOG.md` from `Unreleased` to the release version
  before committing release prep.
- Build the current CLI with `make build` before publishing this repository.
- Publish with `PATH="$PWD/.tmp:$PATH" release-tools publish-tag vX.Y.Z`.
- This intentionally uses the just-built CLI as the release frontend so
  self-release does not depend on an older globally installed binary when
  current config or commands rely on unreleased behavior.
- Ensure `RELEASE_TOKEN`, the native forge token variable, or
  `RELEASE_TOKEN_FILE` is available before publishing.

## Verified Behavior To Preserve
- Keep the installed `release-tools` binary as the only public command surface.
- Keep Make targets maintainer-only; consumer repos should call `release-tools` from `PATH`.
- The CLI fails fast on missing `RELEASE_PROJECT` and `RELEASE_OWNER`; tag publishing also requires `VERSION` or a positional tag.
- `release-tools check` runs `goreleaser check`; when charts are enabled it also
  runs `helm dependency update --skip-refresh` and `helm lint` for each chart.
- `release-tools snapshot` runs `goreleaser release --snapshot --skip=publish
  --clean`; when charts are enabled it also runs `helm package` into
  `dist/charts`.
- `publish` and `publish-tag` package charts before GoReleaser publish starts;
  when `RELEASE_HELM_OCI_REPOSITORY` is set they push packaged charts with
  `helm push` after GoReleaser succeeds.
- `publish-tag` publishes from a clean temporary clone of the exact tag.
- GoReleaser must run from the release repository root.
- unset `RELEASE_ARTIFACTS` keeps current binaries-only behavior.
- `check` and `snapshot` paths must not require `RELEASE_TOKEN`.
- CLI release notes currently support `RELEASE_NOTES_MODE=news-md`, `gnu-news`,
  and `none`.
- CLI release body patching currently supports `RELEASE_BODY_MODE=patch` and `none`.
- project Go preflight is required only when `RELEASE_REQUIRE_GO=1`.
- `VERSION` is the only supported tag override variable; `TAG` is not public config.

## Tooling / Env Notes
- the CLI requires a resolvable `goreleaser`.
- release body patching uses the Go HTTP client.
- token resolution reads `RELEASE_TOKEN`, the native GoReleaser token variable
  for `RELEASE_FORGE`, or `RELEASE_TOKEN_FILE` in that order.
- GoReleaser resolution checks `GORELEASER_BIN`, then common install locations.
- Helm is required only when `RELEASE_ARTIFACTS` includes `charts`.
- Go baseline is Go 1.26 with toolchain `go1.26.4`.
- Dev-container verification uses Podman through `scripts/in-container`; the dev
  container is the source of required development tools, including Helm.

## Editing Notes
- When changing documented behavior, update the matching docs in `docs/usage.md` and `docs/agent-release-flow.md`.
- Prefer executable sources over prose if they conflict.
- Do not add consumer-repo assumptions that are not enforced by this toolkit.
- Do not add Make as a consumer release frontend.
