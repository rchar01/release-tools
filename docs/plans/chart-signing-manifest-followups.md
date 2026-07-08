# Plan: Chart Signing And Manifest Follow-Ups

## Goal

Finish the next layer of chart/signing release work after the stable Helm chart
orchestration, classic provenance signing, and chart release manifest support.
The outcome should improve downstream verification without expanding the public
config surface before behavior is proven.

## Scope

- OCI Helm chart signing by immutable registry digest.
- Optional GoReleaser artifact metadata merging into `dist/release-manifest.json`.
- Optional upload of `dist/release-manifest.json` as a forge release asset.
- Release-readiness validation for the current chart and signing features.

## Non-Goals

- Do not replace GoReleaser as the owner of binary, archive, checksum, container,
  or GoReleaser-supported signing behavior.
- Do not promote OCI chart signing until it signs immutable digests, not mutable
  tags.
- Do not silently reuse forge release tokens for chart registries or package
  registries.
- Do not add Make as a consumer release frontend; Make targets remain
  maintainer-only for this repository.

## Current Context

- Chart-enabled flows support local Helm checks, snapshot packaging, publish-time
  packaging, OCI pushes, ChartMuseum-compatible classic uploads, and Helm
  classic provenance signing.
- Chart-enabled `snapshot`, `publish`, and `publish-tag` write
  `dist/release-manifest.json` with chart package paths, SHA-256 values,
  optional provenance file paths/SHA-256 values, and configured chart registry
  targets.
- `make helm-provenance-test` builds the current CLI, generates a disposable GPG
  key, runs chart-enabled `release-tools snapshot`, and verifies the signed chart
  with `helm verify`.
- GoReleaser metadata from `dist/artifacts.json` is merged into the release
  manifest when present.
- The release manifest can be uploaded as a release asset when
  `RELEASE_MANIFEST_UPLOAD=1` is set.

## Assumptions

- Helm and OCI registry behavior can differ by Helm version and registry
  implementation, so digest capture and signing must be tested against at least
  one real OCI target before becoming stable.
- GoReleaser's artifact metadata schema may change across versions, so manifest
  merging should tolerate missing or unrecognized fields.
- Release asset upload should happen only after all publish-time chart pushes,
  classic uploads, and manifest generation succeed.

## Open Questions

- [x] Which signing tool should be the first stable OCI Helm chart signing path:
  Cosign, `helm-sigstore`, Notation, or another registry-native tool?
  Decision: support Cosign and Notation as digest-signing backends; do not make
  `helm-sigstore` the stable path.
- [x] Which registry should be the first required OCI signing prototype target:
  local Zot, Codeberg package registry if OCI is available, GHCR, or another
  maintained registry?
  Decision: use Helm's digest output from local Zot-backed OCI push/pull testing
  as the first implementation target; live OCI signing against additional
  registries remains useful validation but is not required for the command model.
- [x] Should binary artifact manifest merging be enabled automatically when
  `dist/artifacts.json` exists, or guarded behind an explicit config key?
  Decision: merge automatically when GoReleaser writes `dist/artifacts.json`;
  this is read-only metadata and does not expand the artifact config surface.
- [x] Should manifest release-asset upload be automatic for chart-enabled flows,
  or opt-in to avoid changing release asset surfaces unexpectedly?
  Decision: upload is opt-in with `RELEASE_MANIFEST_UPLOAD=1`.

## Phase 1: Release-Readiness Validation

Goal: Prove the current chart and signing behavior is safe enough for the next
release before adding more features.

Tasks:

- [x] Run `make verify` and record the result.
  Result: passed on 2026-07-08.
- [x] Run `make container-test` and record the result.
  Result: passed on 2026-07-08.
- [x] Run `make helm-registry-test` and record the result.
  Result: passed on 2026-07-08 against local Zot and ChartMuseum; OCI
  push/pull reported digest
  `sha256:1af7724dea16df6403ded78d6f78c054a976e8a72956344d2c6761c35a19a8fa`.
- [x] Run `make helm-provenance-test` and record the result.
  Result: passed on 2026-07-08 with disposable GPG-backed chart verification.
- [x] Run `make codeberg-smoke-test` with package-registry-capable credentials
  and record whether release creation, body patching, and chart upload pass.
  Result: passed on 2026-07-08 for `rch/release-tools-smoke`
  `v0.0.1783530033`; GoReleaser created release `10625726`, release body
  patching succeeded, and the smoke confirmed the chart upload.

Validation gate:

- [x] All release-readiness commands pass, or failures are documented with a
  release decision.

Decision point: Validation passed on 2026-07-08. Proceed with release prep before
new chart/signing feature work.

## Phase 2: OCI Helm Chart Signing Prototype

Goal: Determine whether OCI chart signing can be made stable without signing
mutable tags.

Tasks:

- [x] Research current Helm, OCI registry, and candidate signing-tool behavior
  for chart digest discovery after `helm push`.
- [x] Prototype pushing a chart to an OCI registry and capturing the immutable
  chart digest reliably.
- [x] Prototype signing the pushed chart by digest with the chosen signing tool.
- [ ] Prototype verification from a clean environment using only registry
  contents, public trust material, and documented commands.
- [x] Record registry/tool versions and exact command evidence in this plan.

Validation gate:

- [ ] A pushed OCI chart can be verified by immutable digest after a fresh pull or
  registry lookup.
- [x] Failure behavior is clear when the registry rejects duplicate chart
  versions, missing signatures, or unsupported artifact references.

Decision point: Digest capture is reliable enough to implement command
orchestration because Helm reports `Pushed:` and `Digest:` after successful OCI
pushes. Full clean-environment signature verification remains a validation task
for registry-specific signer behavior.

## Phase 3: Stable OCI Signing Integration

Goal: Add OCI chart signing only after Phase 2 proves the verification model.

Tasks:

- [x] Define the smallest public config needed for OCI chart signing, if any.
  Stable config: `RELEASE_HELM_OCI_SIGNER=cosign|notation|none` and optional
  non-secret `RELEASE_HELM_OCI_SIGN_ARGS`.
- [x] Add strict config validation and `doctor`/`tools-check` preflights for the
  chosen signing tool.
- [x] Sign OCI charts after successful `helm push` and before manifest upload.
- [x] Record signature references or verification metadata in
  `dist/release-manifest.json`.
- [x] Add unit tests for command construction, missing tools, and error paths.
- [ ] Add an integration or smoke test that signs and verifies against the chosen
  OCI registry.
- [x] Update `README.md`, `docs/usage.md`, `docs/agent-release-flow.md`,
  `AGENTS.md`, and `CHANGELOG.md` only for implemented stable behavior.

Validation gate:

- [x] Focused signing tests pass.
- [ ] `make verify` passes.
- [ ] OCI signing smoke test passes.

## Phase 4: GoReleaser Artifact Manifest Merge

Goal: Include binary/archive/checksum metadata in `dist/release-manifest.json`
without taking ownership away from GoReleaser.

Tasks:

- [x] Inspect GoReleaser `dist/artifacts.json` output for this repository and at
  least one representative consumer fixture.
- [x] Define manifest fields for GoReleaser-owned artifacts, including artifact
  name, path, type, target, and checksum when available.
- [x] Keep merging tolerant of absent `dist/artifacts.json` and unknown artifact
  types.
- [x] Add deterministic manifest tests for binary-only and mixed binaries/charts
  flows.
- [x] Document that GoReleaser remains the artifact source of truth.

Validation gate:

- [x] `dist/release-manifest.json` remains deterministic and backward-compatible
  for existing chart consumers.
- [ ] `make verify` passes.

Decision point: Automatic merge is acceptable because it only reflects
GoReleaser-owned metadata already written into `dist/artifacts.json`.

## Phase 5: Release Manifest Asset Upload

Goal: Publish `dist/release-manifest.json` as a release asset after all release
artifacts are final.

Tasks:

- [x] Decide whether upload is automatic for manifest-producing flows or opt-in.
- [x] Design upload sequencing so failed chart publishing or signing cannot leave
  a release asset that claims success.
- [x] Implement upload for supported forges using existing token resolution.
- [x] Define duplicate asset behavior: fail, replace, or verify identical bytes.
  Decision: fail on duplicate or any other non-2xx upload/link response.
- [x] Add tests for upload success, duplicate handling, and API failures.
- [x] Document asset upload behavior and failure modes.

Validation gate:

- [x] Upload tests pass for supported forge APIs.
- [ ] A live smoke test confirms the manifest appears as a release asset only
  after all configured artifact steps succeed.
- [ ] `make verify` passes.

## Risks

- OCI chart signing tools may sign registry-specific artifact references in ways
  that do not round-trip across registries.
- Helm may not expose pushed chart digests consistently enough for stable signing.
- Manifest merging could accidentally depend on unstable GoReleaser metadata
  fields.
- Manifest asset upload could create partial-release confusion if upload happens
  before chart publishing or signing is complete.
- Expanding config too early could lock in public behavior before validation is
  complete.

## Validation Commands

- [x] `make verify`
- [x] `make container-test`
- [x] `make helm-registry-test`
- [x] `make helm-provenance-test`
- [x] `make codeberg-smoke-test`

## Progress Log

| Date | Update | Evidence |
| --- | --- | --- |
| 2026-07-08 | Plan created for chart signing and manifest follow-ups. | User requested a written plan for remaining chart/signing future work. |
| 2026-07-08 | Phase 1 release-readiness validation passed. | `make verify`, `make container-test`, `make helm-registry-test`, `make helm-provenance-test`, and `make codeberg-smoke-test` all passed sequentially; Codeberg smoke passed for `rch/release-tools-smoke` `v0.0.1783530033`. |
| 2026-07-08 | Phase 2 and Phase 3 OCI signing implementation started. | Research selected Cosign and Notation over `helm-sigstore`; implementation parses Helm `Pushed:`/`Digest:` output, signs digest refs only, and records digest/signature metadata in the chart manifest. |
| 2026-07-08 | Phase 4 GoReleaser artifact manifest merge implemented. | Snapshot, publish, and publish-tag now merge `dist/artifacts.json` metadata into `dist/release-manifest.json` when present, including binary-only publish-tag output copying. |
| 2026-07-08 | Phase 5 manifest upload implemented. | `RELEASE_MANIFEST_UPLOAD=1` uploads `dist/release-manifest.json` after all configured publish-time artifact steps succeed; Gitea/Forgejo/Codeberg, GitHub, and GitLab upload paths have focused tests. |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-07-08 | Keep OCI chart signing deferred until immutable digest signing is proven. | Existing stable support covers Helm classic provenance; OCI signing by mutable tag would provide weak verification. |
| 2026-07-08 | Support Cosign and Notation for OCI chart digest signing. | Both tools sign OCI artifact digest references directly; keeping signer choice explicit avoids registry-specific assumptions. |
