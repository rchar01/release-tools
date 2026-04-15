# Examples

These files are ready-to-copy starting points for a Make-only runtime-bootstrap
consumer of `release-tools`.

Suggested usage:

1. copy `bootstrap-release-tools.sh` to `scripts/bootstrap-release-tools.sh`
2. copy the contents of `Makefile.release-tools` into the consumer `Makefile`
3. add `.release-tools-version` at the repository root
4. adapt `.goreleaser.yaml`, `NEWS.md`, and CI workflow details for the project

Files:

- `bootstrap-release-tools.sh`: fetches the pinned toolkit checkout
- `Makefile.release-tools`: example consumer Makefile integration
- `.release-tools-version`: example pinned toolkit tag
- `forgejo-release.yml`: example release workflow
