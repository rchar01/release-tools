# AGENTS.md

## Scope
- This repo is a shared release toolkit for Go, shell, and documentation/toolkit projects.
- Consumer repos are expected to bootstrap a pinned checkout such as `.tmp/release-tools/current`.
- Keep repo-specific release config in the consumer repo; keep shared release behavior in the CLI.
- The CLI is the only public command surface for v2+.
- Do not reintroduce Make wrappers or Make-based integration docs.

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
- `bin/release-tools`
- `cmd/release-tools/main.go`
- `bin/common.sh`
- `bin/run-goreleaser.sh`
- `.release-tools.env`
- `.goreleaser.yaml`
- `scripts/test`
- `scripts/test-errors`

## Repo Shape
- `cmd/release-tools/`: Go CLI source of truth for public release behavior
- `bin/`: compatibility wrappers and private shell helpers
- `.release-tools.env`: self-release config and toolkit version pin for this repo
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
  - `RELEASE_TOOLS_VERSION`
  - `RELEASE_OWNER`
  - `RELEASE_REPO`
  - `RELEASE_API_URL`
  - `RELEASE_DOWNLOAD_URL`
  - `RELEASE_NOTES_SOURCE`
  - `RELEASE_NOTES_MODE`
  - `RELEASE_BODY_MODE`
  - `GORELEASER_CONFIG`
  - `GORELEASER_BIN`
  - `RELEASE_REQUIRE_GO`
  - `VERSION`
- `.release-tools.env` is the default repo-local config file.
- Environment variables override `.release-tools.env` values.
- `CODEBERG_TOKEN` is the only public token variable.
- The CLI maps `CODEBERG_TOKEN` to `GITEA_TOKEN` internally for GoReleaser.

## Commands
- CLI:
  - `bin/release-tools tools-check`
  - `bin/release-tools doctor`
  - `bin/release-tools check`
  - `bin/release-tools snapshot`
  - `bin/release-tools publish`
  - `bin/release-tools publish-tag vX.Y.Z`
  - `bin/release-tools notes vX.Y.Z`
- Verification:
  - `scripts/test`
  - `scripts/test-errors`
  - `scripts/in-container ./scripts/test`

## Verified Behavior To Preserve
- Keep `bin/release-tools` as the only public command surface.
- Keep `bin/release-tools` as the compatibility wrapper for the Go CLI.
- Reuse `bin/ensure-tools.sh` for tool checks instead of duplicating command checks in frontends.
- The CLI fails fast on missing `RELEASE_PROJECT` and `RELEASE_OWNER`; tag publishing also requires `VERSION` or a positional tag.
- `release-tools check` runs `goreleaser check`.
- `release-tools snapshot` runs `goreleaser release --snapshot --skip=publish --clean`.
- `publish-tag` publishes from a clean temporary clone of the exact tag while running the current bootstrapped toolkit against that clone.
- GoReleaser must run from the release repository root.
- `check` and `snapshot` paths must not require `CODEBERG_TOKEN`.
- `release-notes.sh` currently supports `RELEASE_NOTES_MODE=news-md` and `none`.
- `update-release-body.sh` currently supports `RELEASE_BODY_MODE=patch` and `none`.
- project Go preflight is required only when `RELEASE_REQUIRE_GO=1`.

## Tooling / Env Notes
- the CLI requires a resolvable `goreleaser`.
- release body patching uses the Go HTTP client.
- token resolution reads `CODEBERG_TOKEN` or `~/.config/codeberg/token`.
- GoReleaser resolution checks `GORELEASER_BIN`, then common install locations.
- Dev-container verification uses Podman through `scripts/in-container`.

## Editing Notes
- When changing documented behavior, update the matching docs in `docs/usage.md` and `docs/agent-release-flow.md`.
- Prefer executable sources over prose if they conflict.
- Do not add consumer-repo assumptions that are not enforced by this toolkit.
- Do not add Make as a release frontend.
