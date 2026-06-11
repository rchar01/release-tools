# Examples

These files are ready-to-copy starting points for a CLI-only runtime-bootstrap
consumer of `release-tools`.

Suggested usage:

1. copy `bootstrap-release-tools.sh` to `scripts/bootstrap-release-tools.sh`
2. copy `.release-tools.env` to the consumer repository root
3. adapt `.goreleaser.yaml`, `NEWS.md`, and CI workflow details for the project

Files:

- `bootstrap-release-tools.sh`: fetches the pinned toolkit archive or checkout
- `.release-tools.env`: example release-tools CLI config and toolkit version pin
- `forgejo-release.yml`: example release workflow
