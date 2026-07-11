# Self-Release Procedure

This procedure is for maintainers releasing this `release-tools` repository. It
is not the consumer release workflow for downstream repositories.

The root `Makefile` is maintainer-only convenience for verification and local
builds. Publishing still goes through the `release-tools` CLI.

## Before Tagging

1. Update `NEWS.md` and `CHANGELOG.md` from `Unreleased` to the target release
   version.
2. Run the default verification suite:

   ```bash
   make verify
   ```

3. Run the dev-container verification suite:

   ```bash
   make container-test
   ```

4. Run focused smoke tests when the release changes matching behavior:

   ```bash
   make helm-registry-test
   make helm-oci-signing-test
   make helm-provenance-test
   ```

5. Run the live Codeberg smoke test only with a token for the disposable
   `rch/release-tools-smoke` repository:

   ```bash
   make codeberg-smoke-test
   ```

Live smoke-test tokens must stay outside the dev-container build context and be
mounted only at container run time. The host `realpath` tool is required for
token-path validation.

## Tag The Release

After committing and pushing release prep, create and push the annotated tag.
Replace `<release-remote>` with the maintainer's release remote name:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push <release-remote> main vX.Y.Z
```

## Publish The Release

Build the current CLI before publishing this repository:

```bash
make build
```

Ensure `RELEASE_TOKEN`, the native forge token variable, or environment-only
`RELEASE_TOKEN_FILE` is available. For local use, prefer an operator-controlled
environment value such as:

```bash
export RELEASE_TOKEN_FILE=~/.config/forge/token
```

Publish with the just-built CLI by direct path:

```bash
./.tmp/release-tools publish-tag vX.Y.Z
```

This avoids depending on an older globally installed binary while also avoiding
`PATH` trust in every executable under the repo-local `.tmp` directory during a
privileged publish.

## After Publishing

Verify the Codeberg release page has the expected release assets, checksums, and
release body. For chart-related releases, also verify chart registry state and
the generated release manifest when manifest upload is enabled.
