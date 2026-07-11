# Examples

These files are ready-to-copy starting points for an installed-CLI consumer of
`release-tools`.

Suggested usage:

1. install `release-tools` into a directory on `PATH`
2. for binary-only projects, copy `.release-tools.env` to the repository root
3. for Helm chart projects, copy `chart-release.env` as `.release-tools.env`
4. adapt `.goreleaser.yaml`, `NEWS.md`, and CI workflow details for the project
5. follow [`../docs/release-procedures.md`](../docs/release-procedures.md) for
   container and chart scenarios

Files:

- `.release-tools.env`: example release-tools CLI config
- `chart-release.env`: example config for binaries plus Helm chart artifacts
- `forgejo-release.yml`: example release workflow
