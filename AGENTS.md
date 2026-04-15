# AGENTS.md

## Scope
- This repo is a shared release toolkit for Go projects.
- Consumer repos are expected to bootstrap a pinned checkout such as `.tmp/release-tools/current`.
- Keep repo-specific release config in the consumer repo; keep shared release behavior in `bin/`.

## Agent Workflow Expectations
- Read relevant code before editing
- Prefer minimal changes that match existing patterns
- Use a verification-focused subagent for non-trivial test runs or runtime-backed checks
- Use a review-focused subagent after substantial edits to catch regressions and doc/code drift
- Use a research-focused subagent when behavior depends on external tooling or upstream docs
- Summarize any subagent findings you rely on
- Do not revert unrelated worktree changes

## Read First
- `README.md`
- `docs/usage.md`
- `docs/agent-release-flow.md`
- `make/release-tools.mk`
- `bin/common.sh`
- `bin/run-goreleaser.sh`

## Repo Shape
- `bin/`: source of truth for release behavior
- `make/release-tools.mk`: shared Make frontend for consumer repos
- `docs/usage.md`: public integration contract
- `docs/agent-release-flow.md`: rationale and invariants for the release flow

## Public Contract
- Supported caller-provided variables are:
  - `RELEASE_PROJECT`
  - `RELEASE_OWNER`
  - `RELEASE_REPO`
  - `RELEASE_API_URL`
  - `RELEASE_DOWNLOAD_URL`
  - `RELEASE_NOTES_SOURCE`
  - `RELEASE_NOTES_MODE`
  - `RELEASE_BODY_MODE`
  - `GORELEASER_CONFIG`
  - `VERSION`
- `CODEBERG_TOKEN` is the only public token variable.
- `bin/run-goreleaser.sh` maps `CODEBERG_TOKEN` to `GITEA_TOKEN` internally for GoReleaser.

## Commands
- Make frontend:
  - `make -f make/release-tools.mk release-tools-check`
  - `make -f make/release-tools.mk release-check`
  - `make -f make/release-tools.mk release-snapshot`
  - `make -f make/release-tools.mk release`
  - `make -f make/release-tools.mk release-tag VERSION=vX.Y.Z`
  - `make -f make/release-tools.mk release-notes VERSION=vX.Y.Z`

## Verified Behavior To Preserve
- Keep Make as a thin wrapper over `bin/*.sh`.
- Reuse `bin/ensure-tools.sh` for tool checks instead of duplicating command checks in frontends.
- The Make frontend fails fast on missing `RELEASE_PROJECT` and `RELEASE_OWNER`; `release-tag` also requires `VERSION`.
- `release-check` runs `goreleaser build --snapshot --clean`; it is the validation path used here.
- `release-tag.sh` publishes from a clean temporary clone of the exact tag while running the current bootstrapped toolkit against that clone through `RELEASE_REPO_ROOT`.
- `run-goreleaser.sh` must `cd "$REPO_ROOT"` before executing Goreleaser.
- `release-notes.sh` currently supports `RELEASE_NOTES_MODE=news-md` and `none`.
- `update-release-body.sh` currently supports `RELEASE_BODY_MODE=patch` and `none`.

## Tooling / Env Notes
- `bin/ensure-tools.sh` requires `go` and a resolvable `goreleaser`.
- `bin/update-release-body.sh` requires `curl`; it uses `jq` if present and falls back to `python3`.
- `resolve_token()` reads `CODEBERG_TOKEN` or `~/.config/codeberg/token`.
- `resolve_goreleaser_bin()` checks `GORELEASER_BIN`, then common install locations.

## Editing Notes
- When changing documented behavior, update the matching docs in `docs/usage.md` and `docs/agent-release-flow.md`.
- Prefer executable sources over prose if they conflict.
- Do not add consumer-repo assumptions that are not enforced by this toolkit.
