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
- GoReleaser metadata is currently separate in `dist/artifacts.json` and is not
  merged into the release manifest.
- The release manifest is generated locally but is not uploaded as a release
  asset.

## Assumptions

- Helm and OCI registry behavior can differ by Helm version and registry
  implementation, so digest capture and signing must be tested against at least
  one real OCI target before becoming stable.
- GoReleaser's artifact metadata schema may change across versions, so manifest
  merging should tolerate missing or unrecognized fields.
- Release asset upload should happen only after all publish-time chart pushes,
  classic uploads, and manifest generation succeed.

## Open Questions

- [ ] Which signing tool should be the first stable OCI Helm chart signing path:
  Cosign, `helm-sigstore`, Notation, or another registry-native tool?
- [ ] Which registry should be the first required OCI signing prototype target:
  local Zot, Codeberg package registry if OCI is available, GHCR, or another
  maintained registry?
- [ ] Should binary artifact manifest merging be enabled automatically when
  `dist/artifacts.json` exists, or guarded behind an explicit config key?
- [ ] Should manifest release-asset upload be automatic for chart-enabled flows,
  or opt-in to avoid changing release asset surfaces unexpectedly?

## Phase 1: Release-Readiness Validation

Goal: Prove the current chart and signing behavior is safe enough for the next
release before adding more features.

Tasks:

- [ ] Run `make verify` and record the result.
- [ ] Run `make container-test` and record the result.
- [ ] Run `make helm-registry-test` and record the result.
- [ ] Run `make helm-provenance-test` and record the result.
- [ ] Run `make codeberg-smoke-test` with package-registry-capable credentials
  and record whether release creation, body patching, and chart upload pass.

Validation gate:

- [ ] All release-readiness commands pass, or failures are documented with a
  release decision.

Decision point: If validation passes, proceed with release prep. If validation
fails, fix regressions before new chart/signing feature work.

## Phase 2: OCI Helm Chart Signing Prototype

Goal: Determine whether OCI chart signing can be made stable without signing
mutable tags.

Tasks:

- [ ] Research current Helm, OCI registry, and candidate signing-tool behavior
  for chart digest discovery after `helm push`.
- [ ] Prototype pushing a chart to an OCI registry and capturing the immutable
  chart digest reliably.
- [ ] Prototype signing the pushed chart by digest with the chosen signing tool.
- [ ] Prototype verification from a clean environment using only registry
  contents, public trust material, and documented commands.
- [ ] Record registry/tool versions and exact command evidence in this plan.

Validation gate:

- [ ] A pushed OCI chart can be verified by immutable digest after a fresh pull or
  registry lookup.
- [ ] Failure behavior is clear when the registry rejects duplicate chart
  versions, missing signatures, or unsupported artifact references.

Decision point: Promote OCI chart signing only if digest capture and verification
are reliable. Otherwise keep it deferred and document why.

## Phase 3: Stable OCI Signing Integration

Goal: Add OCI chart signing only after Phase 2 proves the verification model.

Tasks:

- [ ] Define the smallest public config needed for OCI chart signing, if any.
- [ ] Add strict config validation and `doctor`/`tools-check` preflights for the
  chosen signing tool.
- [ ] Sign OCI charts after successful `helm push` and before manifest upload.
- [ ] Record signature references or verification metadata in
  `dist/release-manifest.json`.
- [ ] Add unit tests for command construction, missing tools, and error paths.
- [ ] Add an integration or smoke test that signs and verifies against the chosen
  OCI registry.
- [ ] Update `README.md`, `docs/usage.md`, `docs/agent-release-flow.md`,
  `AGENTS.md`, and `CHANGELOG.md` only for implemented stable behavior.

Validation gate:

- [ ] Focused signing tests pass.
- [ ] `make verify` passes.
- [ ] OCI signing smoke test passes.

## Phase 4: GoReleaser Artifact Manifest Merge

Goal: Include binary/archive/checksum metadata in `dist/release-manifest.json`
without taking ownership away from GoReleaser.

Tasks:

- [ ] Inspect GoReleaser `dist/artifacts.json` output for this repository and at
  least one representative consumer fixture.
- [ ] Define manifest fields for GoReleaser-owned artifacts, including artifact
  name, path, type, target, and checksum when available.
- [ ] Keep merging tolerant of absent `dist/artifacts.json` and unknown artifact
  types.
- [ ] Add deterministic manifest tests for binary-only and mixed binaries/charts
  flows.
- [ ] Document that GoReleaser remains the artifact source of truth.

Validation gate:

- [ ] `dist/release-manifest.json` remains deterministic and backward-compatible
  for existing chart consumers.
- [ ] `make verify` passes.

Decision point: Decide whether automatic merge is acceptable or whether it needs
an explicit opt-in key.

## Phase 5: Release Manifest Asset Upload

Goal: Publish `dist/release-manifest.json` as a release asset after all release
artifacts are final.

Tasks:

- [ ] Decide whether upload is automatic for manifest-producing flows or opt-in.
- [ ] Design upload sequencing so failed chart publishing or signing cannot leave
  a release asset that claims success.
- [ ] Implement upload for supported forges using existing token resolution.
- [ ] Define duplicate asset behavior: fail, replace, or verify identical bytes.
- [ ] Add tests for upload success, duplicate handling, and API failures.
- [ ] Document asset upload behavior and failure modes.

Validation gate:

- [ ] Upload tests pass for supported forge APIs.
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

- [ ] `make verify`
- [ ] `make container-test`
- [ ] `make helm-registry-test`
- [ ] `make helm-provenance-test`
- [ ] `make codeberg-smoke-test`

## Progress Log

| Date | Update | Evidence |
| --- | --- | --- |
| 2026-07-08 | Plan created for chart signing and manifest follow-ups. | User requested a written plan for remaining chart/signing future work. |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-07-08 | Keep OCI chart signing deferred until immutable digest signing is proven. | Existing stable support covers Helm classic provenance; OCI signing by mutable tag would provide weak verification. |
