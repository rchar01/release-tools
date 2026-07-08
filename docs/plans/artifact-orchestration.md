# Plan: Artifact Orchestration

## Goal

Expand `release-tools` from a GoReleaser-focused release helper into a small
artifact-aware orchestrator for binaries, container-image preflights, and Helm
charts while keeping project-specific build, image, chart, and signing config in
consumer repositories.

## Scope

- Keep the installed `release-tools` CLI as the only public command surface.
- Keep GoReleaser as the owner of binaries, archives, checksums, release assets,
  container image builds, container signing, and GoReleaser-supported SBOMs.
- Add Helm chart orchestration for check, snapshot, publish, and publish-tag
  workflows.
- Add registry and package-token handling without silently reusing forge release
  tokens for unrelated registries.
- Add signing checks and signing orchestration only after chart publishing is
  stable.
- Add a release manifest only after artifact metadata can be captured reliably.
- Update `README.md`, `docs/usage.md`, `docs/agent-release-flow.md`, examples,
  tests, and `AGENTS.md` as public behavior changes.

## Non-Goals

- Do not replace GoReleaser.
- Do not define Dockerfiles, image names, build matrices, chart content, or SBOM
  policy for consumer repositories.
- Do not make the root `Makefile` a consumer release frontend.
- Do not add a broad config library unless the current strict env-file loader is
  no longer adequate.
- Do not add speculative public config keys before the behavior is implemented.
- Do not implement every Helm backend in the first milestone.
- Do not make Cosign signing for OCI Helm charts stable before it is prototyped
  against a real target registry.

## Current Context

- Current public commands are `version`, `tools-check`, `doctor`, `check`,
  `snapshot`, `publish`, `publish-tag`, `notes`, and `completion`.
- Current release behavior lives mostly in `cmd/release-tools/main.go`.
- Current config loading uses a strict allowlist in `allowedConfigKeys`.
- `publish-tag` publishes from a clean temporary clone of the exact tag.
- `check` runs `goreleaser check`.
- `snapshot` runs `goreleaser release --snapshot --skip=publish --clean`.
- `publish` and `publish-tag` generate release notes, run GoReleaser, then patch
  the forge release body when configured.
- Existing verification commands are `make verify`, `make container-test`, and
  `scripts/test-errors`.

## Assumptions

- Consumer repositories that enable charts have `helm` available locally or in CI.
- Chart version defaults can be derived from release tags by trimming a leading
  `v`, for example `v1.2.3` becomes `1.2.3`.
- GoReleaser remains the only supported container-image publishing path.
- Separate registry/package tokens are needed because forge release tokens and
  package registry tokens may have different scopes.
- Helm publishing and signing behavior must avoid mutating the maintainer's
  global Helm config when possible.

## Open Questions

- [x] Which Helm target should be implemented first after local chart packaging:
  OCI registry or classic Helm package registry?
- [x] Should `release-tools` run `helm registry login` with temporary config, or
  should it require the caller to pre-authenticate for OCI registries?
- [x] Should chart version always come from the release tag, or do we need a
  supported mode where chart version and app version differ?
  Decision: `tag` remains the only stable source for now; reading chart or app
  versions from `Chart.yaml` is future work.
- [x] Is ChartMuseum required, or can classic Helm publishing start with
  Forgejo/Gitea only?
  Decision: ChartMuseum-compatible upload should be supported, including
  Forgejo/Gitea package registries; ChartMuseum is supported but not required.
- [x] Should container-image support be explicit through `RELEASE_ARTIFACTS`, or
  should `release-tools` only detect GoReleaser container config during `doctor`?
  Decision: container image support stays GoReleaser-owned and is detected from
  `.goreleaser.yaml`; there is no `RELEASE_ARTIFACTS=containers` value.

## Proposed Public Config

Initial stable keys for the first Helm milestone:

```sh
RELEASE_ARTIFACTS=binaries,charts
RELEASE_HELM_CHART_DIRS=charts/myapp
RELEASE_HELM_VERSION_FROM=tag
RELEASE_HELM_APP_VERSION_FROM=tag
```

Additional stable keys implemented by later milestones:

```sh
RELEASE_HELM_OCI_REPOSITORY=oci://codeberg.org/myowner/charts
RELEASE_HELM_OCI_USERNAME=robot
RELEASE_HELM_OCI_PASSWORD_FILE=~/.config/registry/token
RELEASE_HELM_OCI_PLAIN_HTTP=0

RELEASE_HELM_CLASSIC_URL=https://codeberg.org/api/packages/myowner/helm
RELEASE_HELM_CLASSIC_USERNAME=robot
RELEASE_HELM_CLASSIC_TOKEN_FILE=~/.config/forgejo/helm-token

RELEASE_HELM_PROVENANCE=false
RELEASE_HELM_GPG_KEY=maintainer@example.org
RELEASE_HELM_GPG_KEYRING=~/.config/helm/release-keyring.gpg
```

Candidate keys for future milestones, not stable until implemented:

```sh
RELEASE_SIGN_MODE=none
RELEASE_COSIGN_MODE=keyless
RELEASE_HELM_OCI_SIGN=false
RELEASE_MANIFEST=false
```

## Design Principles

- Prefer orchestration over reimplementation.
- Validate all configured artifact classes before publishing anything remote.
- Package local artifacts before remote publish steps.
- Keep publish steps idempotent where possible.
- If a remote artifact already exists, verify it matches the expected artifact or
  fail loudly instead of overwriting silently.
- Use temporary directories and temporary Helm registry/config state for release
  work that should not mutate a user's global config.
- Keep config strict and explicit.
- Preserve current error format and existing command behavior unless the release
  notes explicitly document a breaking change.

## Phase 1: Internal Structure For Growth

Goal: Make room for artifact orchestration without turning
`cmd/release-tools/main.go` into an unmaintainable release engine.

Tasks:

- [x] Identify the smallest safe package split for config, command execution,
  GoReleaser invocation, and future Helm logic.
- [x] Move config parsing and env helpers behind testable functions without
  changing public behavior.
- [x] Move external command execution helpers behind a small runner abstraction
  that can be tested without invoking real tools.
- [x] Preserve exact current behavior for GoReleaser commands, notes generation,
  token resolution, and release body patching.
- [x] Update tests to cover moved code at the package boundary.

Validation gate:

- [x] `go test ./cmd/release-tools`
- [x] `scripts/test-errors`
- [x] `make verify`

Decision point: Continue only if the refactor is behavior-preserving and small
enough to review confidently.

## Phase 2: Artifact Class Config

Goal: Add explicit artifact-class configuration without enabling remote chart or
signing behavior yet.

Tasks:

- [x] Add strict config support for `RELEASE_ARTIFACTS`.
- [x] Support `binaries` and `charts` as initial artifact classes.
- [x] Treat unset `RELEASE_ARTIFACTS` as the current binaries-only behavior.
- [x] Reject unknown artifact classes with a clear error.
- [x] Add helper methods for checking whether charts are enabled.
- [x] Update `doctor` output to report enabled artifact classes.
- [x] Add tests for default behavior, comma parsing, whitespace handling, and
  invalid values.

Validation gate:

- [x] `go test ./cmd/release-tools`
- [x] `scripts/test-errors`
- [x] `make verify`

Decision point: Confirm whether container image detection belongs in this config
or remains a GoReleaser preflight concern.

## Phase 3: Local Helm Check And Package

Goal: Support local Helm chart validation and packaging in `check` and
`snapshot` without publishing charts remotely.

Tasks:

- [x] Add strict config support for `RELEASE_HELM_CHART_DIRS`.
- [x] Add strict config support for `RELEASE_HELM_VERSION_FROM=tag`.
- [x] Add strict config support for `RELEASE_HELM_APP_VERSION_FROM=tag`.
- [x] Require `helm` only when charts are enabled.
- [x] Validate chart directories exist when charts are enabled.
- [x] Validate each chart has a readable `Chart.yaml`.
- [x] Derive chart version from the release tag by trimming one leading `v`.
- [x] Run `helm dependency update --skip-refresh <chart>` during chart checks if
  this behavior proves compatible with expected chart dependency workflows.
- [x] Run `helm lint <chart>` during `release-tools check` when charts are
  enabled.
- [x] Run `helm package <chart> --version <version> --app-version <version>
  --destination dist/charts` during `snapshot` when charts are enabled.
- [x] Ensure `snapshot` still does not require publish tokens.
- [x] Add unit tests using a fake command runner.
- [x] Add integration-style tests with stub `helm` and `goreleaser` binaries.

Validation gate:

- [x] `go test ./cmd/release-tools`
- [x] `scripts/test-errors`
- [x] `make verify`
- [x] `make container-test`

Decision point: Review whether local Helm packaging should be released before
remote chart publishing is implemented.

## Phase 4: Publish Sequencing For Charts

Goal: Wire chart packaging into `publish` and `publish-tag` safely before any
remote chart upload backend is added.

Tasks:

- [x] Package charts before GoReleaser publish starts.
- [x] Ensure `publish-tag` packages charts from the clean tag clone, not the
  caller's worktree.
- [x] Ensure chart output goes to the tag clone's `dist/charts` or equivalent
  release-local directory.
- [x] Fail before GoReleaser publish if chart validation or packaging fails.
- [x] Add tests proving chart commands run in the expected order.
- [x] Add tests proving `publish-tag` uses clone-local chart paths.

Validation gate:

- [x] `go test ./cmd/release-tools`
- [x] `scripts/test-errors`
- [x] `make verify`
- [x] `make container-test`

Decision point: Choose the first remote chart publishing target.

## Phase 5: First Remote Helm Backend

Goal: Publish packaged charts to one remote backend with clear auth and
idempotency semantics.

Candidate A: Helm OCI registry.

- [x] Add `RELEASE_HELM_OCI_REPOSITORY`.
- [x] Add explicit OCI auth config with `RELEASE_HELM_OCI_USERNAME`,
  `RELEASE_HELM_OCI_PASSWORD_FILE`, and environment-only
  `RELEASE_HELM_OCI_PASSWORD`.
- [x] Use temporary Helm registry config paths for explicit OCI auth.
- [x] Add explicit `RELEASE_HELM_OCI_PLAIN_HTTP` support for disposable or
  otherwise trusted insecure registries.
- [x] Decide whether `release-tools` performs `helm registry login` or requires
  pre-authenticated Helm registry config.
- [x] Run `helm registry login --password-stdin --registry-config` when explicit
  OCI auth is configured.
- [x] Run `helm push <chart>.tgz oci://...` for each packaged chart.
- [x] Keep publish-time chart packages outside GoReleaser's cleaned `dist`
  directory.
- [x] Capture the resulting chart reference when Helm output exposes it
  reliably.
- [x] Define behavior when a chart version already exists.
- [x] Prototype against a local Zot OCI registry before documenting support as
  stable.

Candidate B: ChartMuseum-compatible classic Helm package registry.

- [x] Use `RELEASE_HELM_CLASSIC_URL` as the opt-in switch.
- [x] Add `RELEASE_HELM_CLASSIC_URL`.
- [x] Add `RELEASE_HELM_CLASSIC_USERNAME`,
  `RELEASE_HELM_CLASSIC_TOKEN_FILE`, and environment-only
  `RELEASE_HELM_CLASSIC_TOKEN`.
- [x] Implement upload using the documented package API or a supported Helm
  plugin only after confirming exact behavior.
- [x] Define behavior when a chart version already exists.
- [x] Prototype raw `/api/charts` upload behavior against local ChartMuseum.
- [x] Prototype against ChartMuseum-compatible backends, including Forgejo/Gitea,
  before documenting classic support as stable.

Validation gate:

- [x] Unit tests for auth resolution and command construction.
- [x] Stubbed publish tests for success and command ordering.
- [x] Manual or container-backed end-to-end prototype against the chosen target.
- [x] `make verify`
- [x] `make container-test`
- [x] `make helm-registry-test`
- [x] `make codeberg-smoke-test`

Decision point: Publish docs for only the backend that has passed an end-to-end
prototype.

## Phase 6: Container Image Preflights

Goal: Improve confidence for repos that use GoReleaser container publishing
without making `release-tools` a container builder.

Tasks:

- [x] Detect whether `.goreleaser.yaml` includes container-image configuration
  such as `dockers`, `dockers_v2`, `docker_manifests`, or `docker_signs`.
- [x] Decide whether detection should use lightweight YAML parsing or a simpler
  documented opt-in.
- [x] Check for required container tooling only when container publishing is
  detected or explicitly enabled.
- [x] Check for `cosign` when GoReleaser image signing is configured, or the
  configured static signing command when `docker_signs` sets `cmd`.
- [x] Add docs stating that GoReleaser owns container image build and push.
- [x] Add docs warning that `dockers_v2` uses Docker buildx.

Validation gate:

- [x] Unit tests for detection or opt-in config parsing.
- [x] `go test ./cmd/release-tools`
- [x] `make verify`

Decision point: Decide whether container support needs any public config beyond
preflight checks.

## Phase 7: Signing

Goal: Add signing orchestration only where the verification model is clear.

Tasks:

- [x] Close `RELEASE_SIGN_MODE=none|cosign|notation` as not needed.
  Not needed: the stable public surface is artifact-specific
  `RELEASE_HELM_OCI_SIGNER=cosign|none` for OCI chart signing,
  `RELEASE_HELM_PROVENANCE` for classic Helm provenance, and GoReleaser config
  for binary and container signing.
- [x] Add `doctor` checks for GPG key/keyring config based on enabled Helm
  provenance behavior.
- [x] Add classic Helm provenance support with `helm package --sign`.
- [x] Add `RELEASE_HELM_PROVENANCE=true|false`.
- [x] Add GPG config keys only when classic provenance is implemented.
- [x] Prototype OCI Helm signing with Cosign against the chosen OCI target
  registry.
  Result: `make helm-oci-signing-test` passed against local Zot on 2026-07-08.
- [x] Sign OCI Helm charts by immutable digest, not just tag, if OCI signing is
  promoted to stable support.
- [x] Keep GoReleaser `docker_signs` as the documented path for container image
  signing.

Validation gate:

- [x] Unit tests for signing config validation.
- [x] Stubbed signing command tests.
- [x] Manual end-to-end signing and verification notes for the chosen backend.
- [x] `make verify`
- [x] `make container-test`

Decision point: Decide whether OCI Helm signing is stable enough for public docs
or should remain experimental guidance.

## Phase 8: Release Manifest

Goal: Write a machine-readable manifest only after artifact metadata is reliable.

Tasks:

- [x] Define `dist/release-manifest.json` schema.
- [x] Record release tag and normalized version.
- [x] Record packaged Helm chart paths, names, versions, and digests.
- [x] Record OCI chart refs when available; OCI digests remain pending until Helm
  exposes them reliably.
- [x] Record classic chart package URLs when available.
- [x] Merge GoReleaser artifact metadata if `dist/artifacts.json` or equivalent
  is available and stable enough.
- [x] Record signatures and provenance files when generated.
- [x] Add optional upload as a forge release asset only after upload behavior is
  designed.

Validation gate:

- [x] Unit tests for manifest schema and deterministic output.
- [x] Snapshot test for a representative release manifest.
- [x] `go test ./cmd/release-tools`
- [x] `make verify`

Decision point: Manifest generation is always on for chart-enabled snapshot and
publish flows, with no new config key. Binary-only manifest generation waits
until GoReleaser artifact metadata merging is designed.

## Phase 9: Documentation And Examples

Goal: Document stable behavior after each milestone, not speculative future
support.

Tasks:

- [x] Update `README.md` with only implemented artifact orchestration behavior.
- [x] Update `docs/usage.md` config contract for each newly supported key.
- [x] Update `docs/agent-release-flow.md` with sequencing and safety invariants.
- [x] Update `AGENTS.md` public contract and verified behavior.
- [x] Add or update examples only for stable workflows.
- [x] Avoid documenting Make as a consumer release frontend.

Validation gate:

- [x] Docs match implemented config allowlist.
- [x] Examples use only released or release-prep behavior.
- [x] `make verify`

## Risks

- Half-published releases if chart publishing starts after GoReleaser publish and
  then fails.
- Registry differences between Forgejo, Gitea, GitHub, GitLab, ChartMuseum, and
  generic OCI registries.
- Helm auth commands mutating user-global config.
- OCI Helm signing behavior differing from container image signing expectations.
- `dockers_v2` upstream behavior changing while it remains provisional.
- Too many config keys becoming public before the behavior is stable.

## Validation Commands

- [x] `go test ./cmd/release-tools`
- [x] `scripts/test-errors`
- [x] `make verify`
- [x] `make container-test`
- [x] `make helm-registry-test`
- [x] `make helm-oci-signing-test`
- [x] `make helm-provenance-test`
- [x] `make codeberg-smoke-test`
- [x] Targeted end-to-end registry prototype commands for each remote backend
  before marking that backend stable.

## Progress Log

| Date | Update | Evidence |
| --- | --- | --- |
| 2026-07-08 | Plan created for artifact orchestration. | User requested a durable implementation plan. |
| 2026-07-08 | Phase 1 refactor implemented. | Added `internal/config` and injectable `internal/runner`; seam-focused tests passed; verification subagent reported `go test ./...`, `scripts/test-errors`, and `make verify` passed. |
| 2026-07-08 | Phase 2 artifact config implemented. | Added `RELEASE_ARTIFACTS` parsing, `chartsEnabled`, `doctor` reporting, focused artifact/doctor tests, and `scripts/test-errors` coverage. Verification subagent reported `go test ./...`, `scripts/test-errors`, and `make verify` passed. |
| 2026-07-08 | Phase 3 local Helm behavior implemented. | Added Helm chart config, local check/package commands, dev-container Helm install, unit tests, `scripts/test-errors`, and stub Helm/GoReleaser integration coverage in `scripts/test`; chart paths are constrained inside the repo including symlink targets; `make verify` and `make container-test` passed. |
| 2026-07-08 | Phase 4 publish sequencing implemented. | Added chart packaging before GoReleaser publish and publish-tag, clone-local packaging tests, failure-ordering coverage, and stub publish coverage in `scripts/test`; `make container-test` passed. |
| 2026-07-08 | Phase 5 OCI backend started. | Added `RELEASE_HELM_OCI_REPOSITORY`, `helm push` after successful GoReleaser publish, unit ordering coverage, and stub publish coverage; local Zot verification later passed. |
| 2026-07-08 | Phase 5 explicit OCI auth implemented. | Added `helm registry login` with `--password-stdin` and temporary registry config when explicit OCI auth is configured; pre-authenticated Helm remains supported when auth config is omitted. |
| 2026-07-08 | Phase 5 classic Helm backend implemented. | Added ChartMuseum-compatible raw chart uploads to `<url>/api/charts` with explicit package token config; local ChartMuseum and live Codeberg package-registry verification later passed. |
| 2026-07-08 | Local Helm registry smoke test implemented. | Added `make helm-registry-test`, `scripts/test-helm-registries`, and `RELEASE_HELM_OCI_PLAIN_HTTP`; smoke test passed against local Zot for OCI push/pull and ChartMuseum for raw `/api/charts` upload. |
| 2026-07-08 | Live Codeberg smoke test started. | Added `make codeberg-smoke-test` for `rch/release-tools-smoke`; live release creation and body patching passed with GoReleaser 2.16.0, and package upload was initially blocked by token package-registry auth (`401 reqPackageAccess`). |
| 2026-07-08 | Live Codeberg chart publishing verified. | `make codeberg-smoke-test` passed with package-registry credentials; the smoke waits for Codeberg Helm `index.yaml` to contain the uploaded chart name and exact release version. |
| 2026-07-08 | Phase 6 container image preflights implemented. | Added top-level GoReleaser container config detection for `dockers`, `dockers_v2`, `docker_manifests`, and `docker_signs`; `doctor` and `tools-check` now require the matching Docker, Podman, Cosign, or configured static signing command only when those keys are present. |
| 2026-07-08 | Phase 8 chart release manifest implemented. | Chart-enabled snapshot, publish, and publish-tag flows now write `dist/release-manifest.json` with tag, version, chart package paths, SHA-256 values, OCI refs, and classic Helm package endpoints; publish commands copy packages back into `dist/charts`, and publish-tag copies chart packages plus the manifest back from the temporary tag clone. |
| 2026-07-08 | Phase 8 GoReleaser artifact metadata merge implemented. | When GoReleaser writes `dist/artifacts.json`, release manifests now include GoReleaser-owned artifact names, types, paths, targets, platforms, and SHA-256 values without changing artifact ownership. |
| 2026-07-08 | Phase 8 manifest asset upload implemented. | `RELEASE_MANIFEST_UPLOAD=1` uploads `dist/release-manifest.json` as a forge release asset after configured artifact publishing/signing succeeds and local manifest generation completes. |
| 2026-07-08 | Phase 7 Helm provenance signing implemented. | Chart-enabled flows can run `helm package --sign` with explicit GPG key/keyring config, copy generated `.prov` files to `dist/charts`, and record provenance paths plus SHA-256 values in `dist/release-manifest.json`. |
| 2026-07-08 | Phase 9 docs and examples pass completed. | Added a stable chart release env example and doc contract tests that compare the canonical usage key list and env examples against the implemented config allowlist. |
| 2026-07-08 | Helm provenance smoke test added and passed. | `make helm-provenance-test` builds the current CLI, generates a disposable GPG key, runs chart-enabled `release-tools snapshot`, confirms manifest provenance metadata, and verifies the signed chart with `helm verify`. |
| 2026-07-08 | OCI chart signing smoke test added and passed. | `make helm-oci-signing-test` builds the current CLI, pushes a chart to local Zot, signs the immutable digest ref with Cosign, confirms manifest signing metadata, and verifies from a clean environment with the public key. |
| 2026-07-08 | Final plan-closure verification passed. | `make verify` passed locally; verification subagent reported `make helm-registry-test`, `make helm-provenance-test`, and `make container-test` all passed sequentially. Live Codeberg smoke verified uploaded manifest asset `release-manifest.json` for `v0.0.1783545390`. |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-07-08 | Keep `release-tools` as an orchestrator rather than a replacement for GoReleaser, Helm, or signing tools. | Preserves current architecture and keeps repo-specific release definitions in consumer repositories. |
| 2026-07-08 | Start with local Helm check/package before remote publishing or signing. | Lowest-risk milestone with clear validation and no registry side effects. |
| 2026-07-08 | Do not silently reuse `RELEASE_TOKEN` for registries and package repositories. | Forge release tokens and registry/package tokens often have different scopes. |
| 2026-07-08 | Implement OCI chart publishing before classic Helm package uploads. | Helm-native `helm push` is the narrowest first remote backend. |
| 2026-07-08 | Keep pre-authenticated Helm supported when explicit OCI auth config is omitted. | Preserves the simple Helm-native path for callers that already manage registry login. |
| 2026-07-08 | Treat existing OCI chart versions as registry-owned errors. | Helm docs do not document overwrite semantics or a force flag, so `release-tools` fails on `helm push` errors rather than trying to overwrite. |
| 2026-07-08 | Support explicit OCI auth without accepting plaintext passwords in `.release-tools.env`. | Keeps committed config free of registry secrets while allowing CI to provide environment-only credentials. |
| 2026-07-08 | Use `RELEASE_HELM_CLASSIC_URL` as the classic Helm opt-in switch. | Avoids an extra mode key while the implemented classic backend is ChartMuseum-compatible, including Forgejo/Gitea package registries. |
| 2026-07-08 | Add explicit plain-HTTP OCI opt-in. | Local Zot smoke testing showed Helm attempts HTTPS by default; `RELEASE_HELM_OCI_PLAIN_HTTP=1` maps directly to Helm's `--plain-http` for registry login and push without making insecure transport implicit. |
| 2026-07-08 | Keep publish chart packages outside `dist`. | A live Codeberg smoke run showed real GoReleaser `--clean` deletes `dist/charts` after pre-publish chart packaging; a temp directory outside the repo preserves fail-before-publish validation without losing packages before upload. |
| 2026-07-08 | Do not add container images to `RELEASE_ARTIFACTS`. | GoReleaser already owns image build, push, manifest, and image-signing behavior; `release-tools` only detects GoReleaser config and preflights local tools. |
| 2026-07-08 | Generate chart manifests without a new config key. | The manifest records chart package metadata only after chart packaging or upload succeeds; broader binary artifact manifests wait for GoReleaser metadata merging. |
| 2026-07-08 | Keep chart and app version source limited to `tag` for current stable behavior. | Tag-derived versions keep binaries and charts aligned; reading `Chart.yaml` versions is useful future work but needs explicit semantics. |
| 2026-07-08 | Treat classic Helm publishing as ChartMuseum-compatible support. | The implemented raw `POST <url>/api/charts` path works for ChartMuseum-style registries, including Forgejo/Gitea package registries, without making ChartMuseum mandatory. |
| 2026-07-08 | Do not add generic `RELEASE_SIGN_MODE`. | A generic signing mode would be ambiguous across binaries, charts, containers, and manifests; artifact-specific signing config keeps GoReleaser-owned signing separate from Helm chart signing. |
