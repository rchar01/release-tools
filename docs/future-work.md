# Future Work

This file tracks useful release-tooling ideas that are not part of the current
stable public contract. Items here are not commitments; they need design,
implementation, and verification before becoming supported behavior.

## Candidate Improvements

- Support reading Helm chart version and app version from `Chart.yaml` with clear
  precedence and release-tag compatibility rules.
- Evaluate Notation or `helm-sigstore` for OCI chart signing after proving
  immutable-digest signing and verification against real registry behavior.
- Add broader registry compatibility smoke tests for OCI and classic Helm package
  registries beyond local Zot, ChartMuseum, and the current live Codeberg smoke.
- Evolve `dist/release-manifest.json` carefully if downstream consumers need
  extra fields, keeping schema changes deterministic and backward-compatible.
- Consider duplicate remote artifact handling that verifies identical bytes before
  failing, instead of always failing on duplicate upload or link responses.
- Document consumer-side installer patterns with a concrete example repository
  when a stable downstream install workflow needs it.

## Not Currently Planned

- A generic `RELEASE_SIGN_MODE` setting. Signing is intentionally
  artifact-specific: GoReleaser owns configured binary and container signing,
  Helm provenance signs packaged chart files, and `RELEASE_HELM_OCI_SIGNER`
  controls OCI chart digest signing.
- A consumer-facing Make release frontend. Consumer repositories should install
  `release-tools` into `PATH` and run the CLI directly.
- Replacing GoReleaser. GoReleaser remains the source of truth for binaries,
  archives, checksums, release assets, container images, and GoReleaser-supported
  signing behavior.
- Reimplementing Helm chart packaging or registry protocols beyond narrow
  orchestration around Helm and documented ChartMuseum-compatible uploads.
