# Installing Cosign

Cosign is required when `RELEASE_HELM_OCI_SIGNER=cosign` is configured or when a
GoReleaser `docker_signs` pipe uses Cosign.

Prefer a trusted OS package manager when available. If downloading directly from
GitHub releases, pin the version and verify the artifact before installing it.
Do not use a just-downloaded, untrusted `cosign` binary to verify itself.

The examples below use Cosign `v3.1.1` for Linux amd64. The artifact names and
SHA-256 digests were checked against the GitHub release API for that tag.

## Linux amd64 Binary With Trusted Cosign

Use this path when you already have a trusted `cosign` on `PATH`, such as one
installed by your OS package manager or provisioned by a trusted CI image.

```bash
COSIGN_VERSION=3.1.1
COSIGN_SHA256=ae1ecd212663f3693ad9edf8b1a183900c9a52d3155ba6e354237f9a0f6463fc
COSIGN_BUNDLE_SHA256=8c2039c784d675f7f8e45c02ad3d658f0bf4d15c05450c354c2b4c31a3597622
COSIGN_BASE_URL="https://github.com/sigstore/cosign/releases/download/v${COSIGN_VERSION}"

curl -fL -o cosign-linux-amd64 \
  "${COSIGN_BASE_URL}/cosign-linux-amd64"
curl -fL -o cosign-linux-amd64.sigstore.json \
  "${COSIGN_BASE_URL}/cosign-linux-amd64.sigstore.json"

printf '%s  %s\n' "$COSIGN_SHA256" cosign-linux-amd64 | sha256sum -c -
printf '%s  %s\n' "$COSIGN_BUNDLE_SHA256" cosign-linux-amd64.sigstore.json | sha256sum -c -

cosign verify-blob cosign-linux-amd64 \
  --bundle cosign-linux-amd64.sigstore.json \
  --certificate-identity keyless@projectsigstore.iam.gserviceaccount.com \
  --certificate-oidc-issuer https://accounts.google.com

sudo install -m 0755 cosign-linux-amd64 /usr/local/bin/cosign
cosign version
```

## Bootstrap Verification Without Existing Cosign

Use this path when you do not already have a trusted `cosign`. It follows the
Sigstore artifact-key verification model documented by the Cosign project and
uses `openssl` to verify the downloaded binary before installing it.

This path requires `go`, `jq`, `openssl`, `curl`, and `base64`. It pins the
`tuf-client` version and checks the downloaded Sigstore root metadata before
using it.

```bash
COSIGN_VERSION=v3.1.1
COSIGN_OS=linux-amd64
COSIGN_SHA256=ae1ecd212663f3693ad9edf8b1a183900c9a52d3155ba6e354237f9a0f6463fc
COSIGN_KMS_BUNDLE_SHA256=60e54a927cf14724d3fc03a2a0fe4cedefd3726ea50e06f2456c9d4dfccfb228
SIGSTORE_ROOT_SHA256=836bff947925edfc23eb9ce17af66fb1e43bb5e2bdd240520985ae52b585eae9
COSIGN_BASE_URL="https://github.com/sigstore/cosign/releases/download/${COSIGN_VERSION}"

go install github.com/theupdateframework/go-tuf/cmd/tuf-client@v0.7.0

curl -fL -o sigstore-root.json \
  https://raw.githubusercontent.com/sigstore/root-signing/refs/heads/main/metadata/root_history/10.root.json
printf '%s  %s\n' "$SIGSTORE_ROOT_SHA256" sigstore-root.json | sha256sum -c -

tuf-client init https://tuf-repo-cdn.sigstore.dev sigstore-root.json
tuf-client get https://tuf-repo-cdn.sigstore.dev artifact.pub > artifact.pub

curl -fL -o cosign-kms.sigstore.json \
  "${COSIGN_BASE_URL}/cosign-${COSIGN_OS}-kms.sigstore.json"
curl -fL -o cosign-linux-amd64 \
  "${COSIGN_BASE_URL}/cosign-${COSIGN_OS}"

printf '%s  %s\n' "$COSIGN_SHA256" cosign-linux-amd64 | sha256sum -c -
printf '%s  %s\n' "$COSIGN_KMS_BUNDLE_SHA256" cosign-kms.sigstore.json | sha256sum -c -

jq -r .messageSignature.signature cosign-kms.sigstore.json | base64 -d \
  > cosign-kms.sig.decoded

openssl dgst -sha256 \
  -verify artifact.pub \
  -signature cosign-kms.sig.decoded \
  cosign-linux-amd64

sudo install -m 0755 cosign-linux-amd64 /usr/local/bin/cosign
cosign version
```

## Debian Or Ubuntu Package

Use a pinned `.deb` release artifact. Verify it before installing with an
already trusted `cosign`. If you do not have one, use the bootstrap binary path
above first, then use that verified `cosign` to verify the package artifact.

```bash
COSIGN_VERSION=3.1.1
COSIGN_DEB_SHA256=b62db813c4e1c47580196aa59e90d0938630c9843d6eea8ae2cc03dcefc00709
COSIGN_DEB_BUNDLE_SHA256=0e51bf4fbb362b722bba30911157121740d2d25b9a7e1b3566f68a83a2a601ea
COSIGN_BASE_URL="https://github.com/sigstore/cosign/releases/download/v${COSIGN_VERSION}"

curl -fL -O "${COSIGN_BASE_URL}/cosign_${COSIGN_VERSION}_amd64.deb"
curl -fL -O "${COSIGN_BASE_URL}/cosign_${COSIGN_VERSION}_amd64.deb.sigstore.json"

printf '%s  %s\n' "$COSIGN_DEB_SHA256" "cosign_${COSIGN_VERSION}_amd64.deb" | sha256sum -c -
printf '%s  %s\n' "$COSIGN_DEB_BUNDLE_SHA256" "cosign_${COSIGN_VERSION}_amd64.deb.sigstore.json" | sha256sum -c -

cosign verify-blob "cosign_${COSIGN_VERSION}_amd64.deb" \
  --bundle "cosign_${COSIGN_VERSION}_amd64.deb.sigstore.json" \
  --certificate-identity keyless@projectsigstore.iam.gserviceaccount.com \
  --certificate-oidc-issuer https://accounts.google.com

sudo dpkg -i "cosign_${COSIGN_VERSION}_amd64.deb"
cosign version
```

## RHEL, Rocky Linux, Or Fedora Package

Use a pinned `.rpm` release artifact. Verify it before installing with an
already trusted `cosign`. If you do not have one, use the bootstrap binary path
above first, then use that verified `cosign` to verify the package artifact.

```bash
COSIGN_VERSION=3.1.1
COSIGN_RPM_SHA256=daa90177c32a62550676ba1cf6be153291d601e53fa0e46a852fc5af020e5674
COSIGN_RPM_BUNDLE_SHA256=b5c279474b2dcc00da7ec9f95c6860294a551256d295b2998b359420a29135c5
COSIGN_BASE_URL="https://github.com/sigstore/cosign/releases/download/v${COSIGN_VERSION}"

curl -fL -O "${COSIGN_BASE_URL}/cosign-${COSIGN_VERSION}-1.x86_64.rpm"
curl -fL -O "${COSIGN_BASE_URL}/cosign-${COSIGN_VERSION}-1.x86_64.rpm.sigstore.json"

printf '%s  %s\n' "$COSIGN_RPM_SHA256" "cosign-${COSIGN_VERSION}-1.x86_64.rpm" | sha256sum -c -
printf '%s  %s\n' "$COSIGN_RPM_BUNDLE_SHA256" "cosign-${COSIGN_VERSION}-1.x86_64.rpm.sigstore.json" | sha256sum -c -

cosign verify-blob "cosign-${COSIGN_VERSION}-1.x86_64.rpm" \
  --bundle "cosign-${COSIGN_VERSION}-1.x86_64.rpm.sigstore.json" \
  --certificate-identity keyless@projectsigstore.iam.gserviceaccount.com \
  --certificate-oidc-issuer https://accounts.google.com

sudo rpm -ivh "cosign-${COSIGN_VERSION}-1.x86_64.rpm"
cosign version
```

## References

- [Official Cosign installation documentation](https://docs.sigstore.dev/cosign/system_config/installation/)
- [Official Cosign verification documentation](https://docs.sigstore.dev/cosign/verifying/verify/)
